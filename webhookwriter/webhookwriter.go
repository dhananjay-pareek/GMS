package webhookwriter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/scrapemate"
)

type ImportRequest struct {
	Entries []*gmaps.Entry `json:"entries"`
}

type webhookWriter struct {
	url    string
	client *http.Client
}

// New returns a scrapemate.ResultWriter that pushes scraped entries to a webhook endpoint.
func New(webhookURL string) scrapemate.ResultWriter {
	if webhookURL == "" {
		webhookURL = os.Getenv("WEBHOOK_URL")
	}
	if webhookURL == "" {
		panic("WEBHOOK_URL not set")
	}

	return &webhookWriter{
		url: webhookURL,
		client: &http.Client{
			Timeout: 120 * time.Second, // Increased for cold starts on free Render tier
		},
	}
}

func (w *webhookWriter) Run(ctx context.Context, in <-chan scrapemate.Result) error {
	const maxBatchSize = 100
	buff := make([]*gmaps.Entry, 0, maxBatchSize)
	lastSave := time.Now().UTC()

	var processData func(any)
	processData = func(data any) {
		switch v := data.(type) {
		case *gmaps.Entry:
			if v != nil {
				buff = append(buff, v)
			}
		case gmaps.Entry:
			buff = append(buff, &v)
		case []*gmaps.Entry:
			for _, e := range v {
				if e != nil {
					buff = append(buff, e)
				}
			}
		case []gmaps.Entry:
			for i := range v {
				buff = append(buff, &v[i])
			}
		case []any:
			// Recursive check for slices of any
			for _, item := range v {
				processData(item)
			}
		}
	}

	for result := range in {
		processData(result.Data)

		if len(buff) >= maxBatchSize || time.Now().UTC().Sub(lastSave) >= time.Minute {
			if err := w.batchSave(ctx, buff); err != nil {
				fmt.Printf("Webhook export error: %v\n", err)
			}
			buff = buff[:0]
			lastSave = time.Now().UTC()
		}
	}

	// Final flush with a fresh context (original may be canceled)
	if len(buff) > 0 {
		// Use a new context with timeout for final send
		finalCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := w.batchSave(finalCtx, buff); err != nil {
			fmt.Printf("Webhook export error: %v\n", err)
		}
	}

	return nil
}

func (w *webhookWriter) batchSave(ctx context.Context, entries []*gmaps.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	reqBody := ImportRequest{Entries: entries}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	// Retry up to 3 times for transient failures
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook request failed (attempt %d): %w", attempt, err)
			fmt.Printf("Webhook attempt %d failed: %v, retrying...\n", attempt, err)
			time.Sleep(time.Duration(attempt*2) * time.Second)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("webhook returned status %d (attempt %d)", resp.StatusCode, attempt)
			fmt.Printf("Webhook attempt %d got status %d, retrying...\n", attempt, resp.StatusCode)
			time.Sleep(time.Duration(attempt*2) * time.Second)
			continue
		}

		if resp.StatusCode >= 400 {
			return fmt.Errorf("webhook returned status code: %d", resp.StatusCode)
		}

		fmt.Printf("Webhook: sent %d entries successfully\n", len(entries))
		return nil
	}

	return lastErr
}
