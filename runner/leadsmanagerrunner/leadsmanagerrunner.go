package leadsmanagerrunner

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/web_leadsmanager"
)

type leadsManagerRunner struct {
	db    *leadsmanager.DB
	srv   *web_leadsmanager.Server
	store leadsmanager.LeadStore
}

func New(ctx context.Context, cfg *runner.Config) (runner.Runner, error) {
	if cfg.RunMode != runner.RunModeLeadsManager {
		return nil, fmt.Errorf("%w: %d", runner.ErrInvalidRunMode, cfg.RunMode)
	}

	localDB, err := leadsmanager.NewDB(ctx, cfg.LeadsDBPath)
	if err != nil {
		return nil, fmt.Errorf("connect to local database: %w", err)
	}

	var store leadsmanager.LeadStore = localDB

	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		supDB, err := leadsmanager.NewSupabaseDB(dsn)
		if err != nil {
			log.Printf("WARNING: could not connect to Supabase (%v) — falling back to local DB only", err)
		} else {
			log.Println("Dual-source mode: fetching leads from local SQLite + Supabase")
			store = leadsmanager.NewCombinedDB(localDB, supDB)
		}
	} else {
		log.Println("Single-source mode: fetching leads from local SQLite only (set DATABASE_URL for Supabase)")
	}

	mgr, err := leadsmanager.NewManager(
		store,
		cfg.LLMProvider,
		cfg.LLMAPIKey,
		cfg.LLMModel,
		cfg.OllamaURL,
	)
	if err != nil {
		localDB.Close()
		return nil, fmt.Errorf("create leads manager: %w", err)
	}

	srv, err := web_leadsmanager.New(
		mgr,
		cfg.LeadsManagerAddr,
		cfg.ScraperURL,
		cfg.LLMProvider,
		cfg.LLMAPIKey,
		cfg.LLMModel,
		cfg.OllamaURL,
	)
	if err != nil {
		localDB.Close()
		return nil, fmt.Errorf("create leads manager server: %w", err)
	}

	return &leadsManagerRunner{
		db:    localDB,
		srv:   srv,
		store: store,
	}, nil
}

func (l *leadsManagerRunner) Run(ctx context.Context) error {
	// Auto-open Leads Manager in app window after server is ready.
	go func() {
		url := "http://localhost" + l.srv.Addr()
		for i := 0; i < 30; i++ {
			time.Sleep(100 * time.Millisecond)
			resp, err := http.Get(url + "/health")
			if err == nil {
				resp.Body.Close()
				break
			}
		}
		openBrowserApp(url)
	}()

	// Start background keep-alive loop for Supabase (every 1 hour)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.store.KeepAlive(ctx); err != nil {
					log.Printf("Background Keep-Alive failed: %v", err)
				} else {
					log.Println("Background Keep-Alive: Supabase connection pinged successfully")
				}
			}
		}
	}()

	return l.srv.Start(ctx)
}

func (l *leadsManagerRunner) Close(context.Context) error {
	if l.db != nil {
		l.db.Close()
	}

	return nil
}

func openBrowserApp(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		edgePath := findEdgePath()
		if edgePath != "" {
			cmd = exec.Command(edgePath, "--app="+url, "--window-size=1200,800")
		} else {
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		}
	case "darwin":
		chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chromePath); err == nil {
			cmd = exec.Command(chromePath, "--app="+url, "--window-size=1200,800")
		} else {
			cmd = exec.Command("open", url)
		}
	default:
		for _, browser := range []string{"google-chrome", "chromium-browser", "chromium"} {
			if path, err := exec.LookPath(browser); err == nil {
				cmd = exec.Command(path, "--app="+url, "--window-size=1200,800")
				break
			}
		}
		if cmd == nil {
			cmd = exec.Command("xdg-open", url)
		}
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open leads manager app window: %v", err)
	}
}

func findEdgePath() string {
	paths := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	chromePaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}

	for _, p := range chromePaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}
