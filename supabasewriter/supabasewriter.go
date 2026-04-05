package supabasewriter

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
	"github.com/gosom/scrapemate"
)

type supabaseWriter struct {
	db *leadsmanager.DB
}

// New creates a ResultWriter that writes directly to Supabase/PostgreSQL.
// This reuses the leadsmanager package for database operations and enrichment.
func New(dbURL string) (scrapemate.ResultWriter, error) {
	if dbURL == "" {
		dbURL = os.Getenv("SUPABASE_DB_URL")
	}
	if dbURL == "" {
		return nil, fmt.Errorf("SUPABASE_DB_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := leadsmanager.NewDB(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect to supabase: %w", err)
	}

	log.Println("Supabase writer: connected successfully")

	return &supabaseWriter{db: db}, nil
}

func (w *supabaseWriter) Run(ctx context.Context, in <-chan scrapemate.Result) error {
	const batchSize = 50
	var buffer []gmaps.Entry

	var processData func(any)
	processData = func(data any) {
		switch v := data.(type) {
		case *gmaps.Entry:
			if v != nil && v.PlaceID != "" {
				buffer = append(buffer, *v)
			}
		case gmaps.Entry:
			if v.PlaceID != "" {
				buffer = append(buffer, v)
			}
		case []*gmaps.Entry:
			for _, e := range v {
				if e != nil && e.PlaceID != "" {
					buffer = append(buffer, *e)
				}
			}
		case []gmaps.Entry:
			for _, e := range v {
				if e.PlaceID != "" {
					buffer = append(buffer, e)
				}
			}
		case []any:
			for _, item := range v {
				processData(item)
			}
		}
	}

	for result := range in {
		processData(result.Data)

		if len(buffer) >= batchSize {
			// Use fresh context for each batch to avoid cancellation issues
			saveCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			if err := w.saveBatch(saveCtx, buffer); err != nil {
				log.Printf("Supabase save error: %v", err)
			}
			cancel()
			buffer = buffer[:0]
		}
	}

	// Final flush with fresh context
	if len(buffer) > 0 {
		finalCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if err := w.saveBatch(finalCtx, buffer); err != nil {
			log.Printf("Supabase final save error: %v", err)
		}
	}

	w.db.Close()
	return nil
}

func (w *supabaseWriter) saveBatch(ctx context.Context, entries []gmaps.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	// Convert entries to leads
	leads := make([]leadsmanager.Lead, 0, len(entries))
	for _, entry := range entries {
		lead := leadsmanager.ProcessEntry(entry)
		leads = append(leads, lead)
	}

	// Bulk upsert all leads in one batch operation
	successCount, err := w.db.BulkUpsertLeads(ctx, leads)
	if err != nil {
		log.Printf("Supabase bulk upsert error: %v", err)
	}

	log.Printf("Supabase: saved %d/%d entries", successCount, len(entries))
	return nil
}
