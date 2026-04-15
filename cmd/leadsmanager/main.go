package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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

	db, err := leadsmanager.NewDB(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	mgr, err := leadsmanager.NewManager(db, llmProvider, llmAPIKey, llmModel, ollamaURL)
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

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
