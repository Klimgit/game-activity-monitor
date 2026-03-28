package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"

	serverapi "game-activity-monitor/server/internal/api"
	"game-activity-monitor/server/internal/auth"
	"game-activity-monitor/server/internal/config"
	"game-activity-monitor/server/internal/storage"
	mig "game-activity-monitor/server/migrations"
)

func main() {
	cfg := config.Load()

	// ── Database ─────────────────────────────────────────────────────────────
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

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

	addr := ":" + cfg.Server.Port
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
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
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
