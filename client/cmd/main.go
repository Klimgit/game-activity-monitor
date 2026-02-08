package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"

	"game-activity-monitor/client/internal/aggregator"
	"game-activity-monitor/client/internal/api"
	"game-activity-monitor/client/internal/collectors"
	"game-activity-monitor/client/internal/config"
	"game-activity-monitor/client/internal/hotkeys"
	"game-activity-monitor/client/internal/models"
	"game-activity-monitor/client/internal/storage"
)

// Dev-only: at most one open interval at a time (FSM aligned with server).
var (
	intervalMu    sync.Mutex
	intervalStart *time.Time
	intervalState string
)

func startIntervalMark(state string) {
	intervalMu.Lock()
	defer intervalMu.Unlock()
	if intervalStart != nil {
		log.Printf("interval: already open (%s), ignoring start %s", intervalState, state)
		return
	}
	t := time.Now().UTC()
	intervalStart = &t
	intervalState = state
	log.Printf("interval: started %s", state)
}

func endIntervalMark(ctx context.Context, client *api.Client, expected string) {
	intervalMu.Lock()
	if intervalStart == nil {
		intervalMu.Unlock()
		log.Printf("interval: end %s ignored (nothing open)", expected)
		return
	}
	if intervalState != expected {
		open := intervalState
		intervalMu.Unlock()
		log.Printf("interval: end %s ignored (open state is %s)", expected, open)
		return
	}
	start := *intervalStart
	intervalStart = nil
	intervalState = ""
	intervalMu.Unlock()
	end := time.Now().UTC()
	if err := client.CreateActivityInterval(ctx, expected, start, end); err != nil {
		log.Printf("interval: %v", err)
	} else {
		log.Printf("interval: recorded %s (%.1fs)", expected, end.Sub(start).Seconds())
	}
}

func main() {
	// systray.Run must be called from the main goroutine.
	// All real work happens inside onReady (called by systray on a separate thread).
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTitle("Game Monitor")
	systray.SetTooltip("Game Activity Monitor — running")

	mStatus := systray.AddMenuItem("Status: idle", "Current monitoring status")
	mStatus.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop monitoring and exit")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	go run(ctx, cancel, mStatus)

	go func() {
		<-mQuit.ClickedCh
		cancel()
		systray.Quit()
	}()
}

func onExit() {
	log.Println("game-monitor: shutting down")
}

func run(ctx context.Context, cancel context.CancelFunc, statusItem *systray.MenuItem) {
	defer cancel()

	// ── Config ────────────────────────────────────────────────────────────────
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// ── Local storage ─────────────────────────────────────────────────────────
	store, err := storage.New("game-monitor.db")
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer func() {
		if cerr := store.Close(); cerr != nil {
			log.Printf("close sqlite: %v", cerr)
		}
	}()

	// ── API client ────────────────────────────────────────────────────────────
	apiClient := api.NewClient(cfg.Server.URL, cfg.Offline.FlushInterval, store)

	if cfg.Auth.Email != "" && cfg.Auth.Password != "" {
		apiClient.SetCredentials(cfg.Auth.Email, cfg.Auth.Password)

		if err := apiClient.Login(ctx, cfg.Auth.Email, cfg.Auth.Password); err != nil {
			log.Printf("login failed: %v — continuing offline, will retry", err)
		} else {
			log.Printf("logged in as %s (user_id=%d)", cfg.Auth.Email, apiClient.UserID())
		}
	}

	go apiClient.StartSyncWorker(ctx)

	rawChan := make(chan *models.RawEvent, cfg.Offline.MaxQueueSize)
	aggChan := make(chan *models.RawEvent, 512)

	mgr := collectors.NewManager(cfg)

	hookCh := mgr.SubscribeHook()

	mgr.Start(ctx, rawChan)

	agg := aggregator.New(cfg.Collectors.Intervals.AggregationWindow, rawChan, aggChan)
	go agg.Run(ctx)

	go forwardEvents(ctx, apiClient, aggChan)

	// ── Hotkeys ───────────────────────────────────────────────────────────────
	hotkeyMgr := hotkeys.NewManagerFromBus(hookCh)
	hotkeys.RegisterAll(hotkeyMgr, cfg.Hotkeys, map[string]func(){
		"start_session": func() {
			if err := apiClient.StartSession(ctx, ""); err != nil {
				log.Printf("start session: %v", err)
			} else {
				log.Println("session started")
				statusItem.SetTitle("Status: gaming")
			}
		},
		"end_session": func() {
			if err := apiClient.EndSession(ctx); err != nil {
				log.Printf("end session: %v", err)
			} else {
				log.Println("session ended")
				statusItem.SetTitle("Status: idle")
			}
		},
		"start_active":  func() { startIntervalMark("active_gameplay") },
		"end_active":    func() { endIntervalMark(ctx, apiClient, "active_gameplay") },
		"start_afk":     func() { startIntervalMark("afk") },
		"end_afk":       func() { endIntervalMark(ctx, apiClient, "afk") },
		"start_menu":    func() { startIntervalMark("menu") },
		"end_menu":      func() { endIntervalMark(ctx, apiClient, "menu") },
		"start_loading": func() { startIntervalMark("loading") },
		"end_loading":   func() { endIntervalMark(ctx, apiClient, "loading") },
	})

	go hotkeyMgr.Start(ctx)

	log.Println("game-monitor running — hotkeys: session ctrl+shift+s/e; intervals ctrl+shift+1..8 (start/end per state)")
	<-ctx.Done()
}

func forwardEvents(ctx context.Context, client *api.Client, ch <-chan *models.RawEvent) {
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := client.Enqueue(ctx, ev); err != nil {
				log.Printf("enqueue event: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
