package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
	"github.com/gosom/google-maps-scraper/web_leadsmanager"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received signal, shutting down Leads Manager...")
		cancel()
	}()

	dbPath := leadsmanager.ResolveDBPath("")

	port := os.Getenv("PORT")
	if port == "" {
		port = ":9090"
	}
	if port[0] != ':' {
		port = ":" + port
	}

	scraperURL := os.Getenv("SCRAPER_URL")
	if scraperURL == "" {
		scraperURL = "http://localhost:8080"
	}

	llmProvider := os.Getenv("LLM_PROVIDER")
	if llmProvider == "" {
		llmProvider = "ollama"
	}
	llmAPIKey := os.Getenv("LLM_API_KEY")
	llmModel := os.Getenv("LLM_MODEL")
	if llmModel == "" {
		llmModel = "qwen3-coder:480b-cloud"
	}
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	// Open local SQLite database
	localDB, err := leadsmanager.NewDB(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to local database: %v\n", err)
		os.Exit(1)
	}
	defer localDB.Close()

	// Build the LeadStore: CombinedDB if DATABASE_URL is set, otherwise local only.
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

	mgr, err := leadsmanager.NewManager(store, llmProvider, llmAPIKey, llmModel, ollamaURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create manager: %v\n", err)
		os.Exit(1)
	}

	srv, err := web_leadsmanager.New(mgr, port, scraperURL, llmProvider, llmAPIKey, llmModel, ollamaURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	log.Printf("Leads Manager starting on http://localhost%s\n", port)

	// Start background keep-alive loop for Supabase (every 1 hour)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.KeepAlive(ctx); err != nil {
					log.Printf("Background Keep-Alive failed: %v", err)
				} else {
					log.Println("Background Keep-Alive: Supabase connection pinged successfully")
				}
			}
		}
	}()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
