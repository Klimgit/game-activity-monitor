package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"

	"game-activity-monitor/client/internal/aggregator"
	"game-activity-monitor/client/internal/api"
	"game-activity-monitor/client/internal/collectors"
	"game-activity-monitor/client/internal/config"
	"game-activity-monitor/client/internal/models"
	"game-activity-monitor/client/internal/storage"
	"game-activity-monitor/client/internal/trayprompt"
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

type trayMenu struct {
	status *systray.MenuItem

	startSession         *systray.MenuItem
	startSessionWithName *systray.MenuItem
	endSession           *systray.MenuItem

	startActive, endActive   *systray.MenuItem
	startAFK, endAFK         *systray.MenuItem
	startMenu, endMenu       *systray.MenuItem
	startLoading, endLoading *systray.MenuItem
}

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTitle("Game Monitor")
	systray.SetTooltip("Game Activity Monitor — running")

	tray := trayMenu{
		status: systray.AddMenuItem("Status: idle", "Current monitoring status"),
	}
	tray.status.Disable()

	systray.AddSeparator()

	sessionRoot := systray.AddMenuItem("Session", "Start or end a gaming session")
	tray.startSession = sessionRoot.AddSubMenuItem(
		"Start session",
		"Uses session.default_game_name from config (can be empty). Set name in the web dashboard anytime.",
	)
	tray.startSessionWithName = sessionRoot.AddSubMenuItem(
		"Start session (enter name)…",
		"Optional: type game name now, or leave empty and edit in the dashboard.",
	)
	tray.endSession = sessionRoot.AddSubMenuItem("End session", "End current session")

	labelsRoot := systray.AddMenuItem("Activity labels", "Mark intervals for ML (one open at a time)")
	ag := labelsRoot.AddSubMenuItem("Active gameplay", "Start/end active gameplay interval")
	tray.startActive = ag.AddSubMenuItem("Start", "")
	tray.endActive = ag.AddSubMenuItem("End", "")

	afk := labelsRoot.AddSubMenuItem("AFK", "Start/end AFK interval")
	tray.startAFK = afk.AddSubMenuItem("Start", "")
	tray.endAFK = afk.AddSubMenuItem("End", "")

	menu := labelsRoot.AddSubMenuItem("Menu", "Start/end in-game menu interval")
	tray.startMenu = menu.AddSubMenuItem("Start", "")
	tray.endMenu = menu.AddSubMenuItem("End", "")

	load := labelsRoot.AddSubMenuItem("Loading", "Start/end loading interval")
	tray.startLoading = load.AddSubMenuItem("Start", "")
	tray.endLoading = load.AddSubMenuItem("End", "")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop monitoring and exit")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	go run(ctx, cancel, &tray)

	go func() {
		<-mQuit.ClickedCh
		cancel()
		systray.Quit()
	}()
}

func onExit() {
	log.Println("game-monitor: shutting down")
}

func listenTrayClick(ctx context.Context, item *systray.MenuItem, fn func()) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-item.ClickedCh:
				go fn()
			}
		}
	}()
}

func run(ctx context.Context, cancel context.CancelFunc, tray *trayMenu) {
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

	// ── Tray menu actions ─────────────────────────────────────────────────────
	listenTrayClick(ctx, tray.startSession, func() {
		name := strings.TrimSpace(cfg.Session.DefaultGameName)
		if err := apiClient.StartSession(ctx, name); err != nil {
			log.Printf("start session: %v", err)
		} else {
			log.Printf("session started (game_name=%q — edit in dashboard if needed)", name)
			tray.status.SetTitle("Status: gaming")
		}
	})
	listenTrayClick(ctx, tray.startSessionWithName, func() {
		def := strings.TrimSpace(cfg.Session.DefaultGameName)
		name, ok, err := trayprompt.GameName(ctx, def)
		if err != nil {
			log.Printf("optional game name dialog: %v", err)
			return
		}
		if !ok {
			return
		}
		name = strings.TrimSpace(name)
		if err := apiClient.StartSession(ctx, name); err != nil {
			log.Printf("start session: %v", err)
		} else {
			log.Printf("session started (game_name=%q)", name)
			tray.status.SetTitle("Status: gaming")
		}
	})
	listenTrayClick(ctx, tray.endSession, func() {
		if err := apiClient.EndSession(ctx); err != nil {
			log.Printf("end session: %v", err)
		} else {
			log.Println("session ended")
			tray.status.SetTitle("Status: idle")
		}
	})
	listenTrayClick(ctx, tray.startActive, func() { startIntervalMark("active_gameplay") })
	listenTrayClick(ctx, tray.endActive, func() { endIntervalMark(ctx, apiClient, "active_gameplay") })
	listenTrayClick(ctx, tray.startAFK, func() { startIntervalMark("afk") })
	listenTrayClick(ctx, tray.endAFK, func() { endIntervalMark(ctx, apiClient, "afk") })
	listenTrayClick(ctx, tray.startMenu, func() { startIntervalMark("menu") })
	listenTrayClick(ctx, tray.endMenu, func() { endIntervalMark(ctx, apiClient, "menu") })
	listenTrayClick(ctx, tray.startLoading, func() { startIntervalMark("loading") })
	listenTrayClick(ctx, tray.endLoading, func() { endIntervalMark(ctx, apiClient, "loading") })

	rawChan := make(chan *models.RawEvent, cfg.Offline.MaxQueueSize)
	aggChan := make(chan *models.RawEvent, 512)

	mgr := collectors.NewManager(cfg)

	mgr.Start(ctx, rawChan)

	agg := aggregator.New(cfg.Collectors.Intervals.AggregationWindow, rawChan, aggChan)
	go agg.Run(ctx)

	go forwardEvents(ctx, apiClient, aggChan)

	log.Println("game-monitor running — use the tray menu: Session and Activity labels")
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
