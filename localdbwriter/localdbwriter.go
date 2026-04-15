package localdbwriter

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
	"github.com/gosom/scrapemate"
)

type writer struct {
	db *leadsmanager.DB
}

func New(ctx context.Context, dbPath string) (scrapemate.ResultWriter, error) {
	db, err := leadsmanager.NewDB(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("connect to local leads db: %w", err)
	}

	return &writer{db: db}, nil
}

func (w *writer) Run(ctx context.Context, in <-chan scrapemate.Result) error {
	const batchSize = 50
	var buffer []gmaps.Entry

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

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

	for {
		select {
		case result, ok := <-in:
			if !ok {
				if len(buffer) > 0 {
					w.saveBatch(context.Background(), buffer)
				}
				w.db.Close()
				return nil
			}
			processData(result.Data)
			if len(buffer) >= batchSize {
				w.saveBatch(ctx, buffer)
				buffer = buffer[:0]
			}
		case <-ticker.C:
			if len(buffer) > 0 {
				w.saveBatch(ctx, buffer)
				buffer = buffer[:0]
			}
		case <-ctx.Done():
			if len(buffer) > 0 {
				w.saveBatch(context.Background(), buffer)
			}
			w.db.Close()
			return ctx.Err()
		}
	}
}

func (w *writer) saveBatch(ctx context.Context, entries []gmaps.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	leads := make([]leadsmanager.Lead, 0, len(entries))
	for _, entry := range entries {
		leads = append(leads, leadsmanager.ProcessEntry(entry))
	}

	successCount, err := w.db.BulkUpsertLeads(ctx, leads)
	if err != nil {
		log.Printf("local leads bulk upsert error: %v", err)
	}

	log.Printf("local leads db: saved %d/%d entries", successCount, len(entries))

	return nil
}
