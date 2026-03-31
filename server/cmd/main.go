package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"

	serverapi "game-activity-monitor/server/internal/api"
	"game-activity-monitor/server/internal/auth"
	"game-activity-monitor/server/internal/config"
	"game-activity-monitor/server/internal/dataset"
	"game-activity-monitor/server/internal/storage"
	mig "game-activity-monitor/server/migrations"
)

func main() {
	cfg := config.Load()

	insecureDefaults := []string{"change-me-in-production", "CHANGE_ME", "secret", ""}
	for _, bad := range insecureDefaults {
		if cfg.Auth.JWTSecret == bad {
			log.Println("WARNING: JWT_SECRET is set to an insecure default value. " +
				"Set the JWT_SECRET environment variable before deploying to production.")
			break
		}
	}

	// ── Database ─────────────────────────────────────────────────────────────
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("close db: %v", cerr)
		}
	}()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	log.Println("connected to database")

	// ── Migrations ────────────────────────────────────────────────────────────
	if err := runMigrations(cfg.Database.URL); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations applied")

	// ── Wiring ────────────────────────────────────────────────────────────────
	store := storage.NewTimescaleStorage(db)
	jwtMgr := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenDuration)
	router := serverapi.SetupRouter(store, jwtMgr)

	if cfg.DatasetAutomation.Enabled {
		if cfg.DatasetAutomation.OutputDir == "" {
			log.Fatal("DATASET_AUTOMATION_ENABLED=true requires DATASET_AUTOMATION_OUTPUT_DIR")
		}
		dataset.StartAutomation(context.Background(), store, dataset.AutomationConfig{
			Enabled:    cfg.DatasetAutomation.Enabled,
			Interval:   cfg.DatasetAutomation.Interval,
			OutputDir:  cfg.DatasetAutomation.OutputDir,
			RunOnStart: cfg.DatasetAutomation.RunOnStart,
		})
	}

	addr := ":" + cfg.Server.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func runMigrations(dbURL string) error {
	src, err := iofs.New(mig.FS, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dbURL)
	if err != nil {
		return err
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Printf("migrate: close source: %v", srcErr)
		}
		if dbErr != nil {
			log.Printf("migrate: close database: %v", dbErr)
		}
	}()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
