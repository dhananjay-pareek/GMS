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
			Timeout: 30 * time.Second,
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

	if len(buff) > 0 {
		if err := w.batchSave(ctx, buff); err != nil {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status code: %d", resp.StatusCode)
	}

	return nil
}
