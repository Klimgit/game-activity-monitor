package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"

	"game-activity-monitor/server/internal/dataset"
	"game-activity-monitor/server/internal/storage"
)

func main() {
	fromStr := flag.String("from", "", "start date YYYY-MM-DD (UTC)")
	toStr := flag.String("to", "", "end date YYYY-MM-DD (UTC)")
	outPath := flag.String("o", "dataset-windows.csv", "output CSV path")
	dsn := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL URL (default: $DATABASE_URL)")
	userID := flag.Int64("user", 0, "if set, export only this user; default: all users")
	sessionIDStr := flag.String("session-id", "", "optional session filter")
	trainingOnly := flag.Bool("training-only", true, "omit windows with no overlapping activity_interval label")
	flag.Parse()

	if *fromStr == "" || *toStr == "" {
		log.Fatal("collectdataset: -from and -to are required (YYYY-MM-DD)")
	}
	if *dsn == "" {
		log.Fatal("collectdataset: set -database-url or DATABASE_URL")
	}

	from, err := time.ParseInLocation("2006-01-02", *fromStr, time.UTC)
	if err != nil {
		log.Fatalf("collectdataset: bad -from: %v", err)
	}
	toDay, err := time.ParseInLocation("2006-01-02", *toStr, time.UTC)
	if err != nil {
		log.Fatalf("collectdataset: bad -to: %v", err)
	}
	to := toDay.Add(24*time.Hour - time.Nanosecond)

	var sessionID *int64
	if *sessionIDStr != "" {
		id, err := strconv.ParseInt(*sessionIDStr, 10, 64)
		if err != nil {
			log.Fatalf("collectdataset: bad -session-id: %v", err)
		}
		sessionID = &id
	}

	db, err := sql.Open("postgres", *dsn)
	if err != nil {
		log.Fatalf("collectdataset: open db: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("collectdataset: close db: %v", cerr)
		}
	}()

	if err := db.PingContext(context.Background()); err != nil {
		log.Fatalf("collectdataset: ping: %v", err)
	}

	st := storage.NewTimescaleStorage(db, nil)
	ctx := context.Background()

	var users []int64
	if *userID != 0 {
		users = []int64{*userID}
	} else {
		users, err = st.ListUserIDs(ctx)
		if err != nil {
			log.Fatalf("collectdataset: list users: %v", err)
		}
	}

	out, err := os.Create(*outPath)
	if err != nil {
		log.Fatalf("collectdataset: create output: %v", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("collectdataset: close output: %v", cerr)
		}
	}()

	for i, uid := range users {
		if err := dataset.WriteDatasetWindowsCSV(ctx, out, st, uid, from, to, sessionID, *trainingOnly, i == 0); err != nil {
			log.Fatalf("collectdataset: user %d: %v", uid, err)
		}
	}

	if _, err := fmt.Fprintf(os.Stderr, "collectdataset: wrote %d user(s) to %s\n", len(users), *outPath); err != nil {
		log.Printf("collectdataset: write status: %v", err)
	}
}
