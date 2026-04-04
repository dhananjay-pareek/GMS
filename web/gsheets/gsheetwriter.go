package gsheets

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/scrapemate"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const maxRowsPerSheet = 180000 // stay well under the ~200K practical limit

type gsWriter struct {
	spreadsheetID string
	baseSheetName string
	srv           *sheets.Service

	mu           sync.Mutex
	buffer       [][]interface{}
	maxSize      int
	currentSheet string
	currentRows  int64
	sheetIndex   int
}

// New creates a new scrapemate.ResultWriter that appends rows to a Google Sheet.
// Reads Service Account credentials from the GOOGLE_CREDENTIALS_JSON env var.
// It auto-writes headers on an empty sheet and creates overflow tabs when full.
func New(spreadsheetID, sheetName string) (scrapemate.ResultWriter, error) {
	ctx := context.Background()

	credsJSON := os.Getenv("GOOGLE_CREDENTIALS_JSON")
	if credsJSON == "" {
		return nil, fmt.Errorf("GOOGLE_CREDENTIALS_JSON environment variable is not set")
	}

	b := []byte(credsJSON)

	config, err := google.JWTConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	client := config.Client(ctx)
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Sheets client: %w", err)
	}

	if sheetName == "" {
		sheetName = "Sheet1"
	}

	ans := &gsWriter{
		spreadsheetID: spreadsheetID,
		baseSheetName: sheetName,
		srv:           srv,
		maxSize:       20, // flush every 20 rows
		currentSheet:  sheetName,
		sheetIndex:    1,
	}

	// Check existing row count and write headers if the sheet is empty
	if err := ans.initSheet(); err != nil {
		return nil, err
	}

	return ans, nil
}

// initSheet checks the current sheet's row count and writes headers if empty.
func (w *gsWriter) initSheet() error {
	resp, err := w.srv.Spreadsheets.Values.Get(w.spreadsheetID, w.currentSheet).Do()
	if err != nil {
		// Sheet might not exist yet; that's fine, we'll try to create it
		log.Printf("Could not read sheet %q (may not exist yet): %v", w.currentSheet, err)
		w.currentRows = 0

		return w.writeHeaders()
	}

	w.currentRows = int64(len(resp.Values))

	if w.currentRows == 0 {
		return w.writeHeaders()
	}

	log.Printf("Sheet %q already has %d rows", w.currentSheet, w.currentRows)

	return nil
}

func (w *gsWriter) writeHeaders() error {
	headers := append((&gmaps.Entry{}).CsvHeaders(), "Scrape_Status")

	headerRow := make([]interface{}, len(headers))
	for i, h := range headers {
		headerRow[i] = h
	}

	vr := &sheets.ValueRange{
		Values: [][]interface{}{headerRow},
	}

	_, err := w.srv.Spreadsheets.Values.Append(w.spreadsheetID, w.currentSheet, vr).
		ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("failed to write headers to %q: %w", w.currentSheet, err)
	}

	w.currentRows = 1
	log.Printf("Wrote headers to sheet %q", w.currentSheet)

	return nil
}

func (w *gsWriter) Run(ctx context.Context, in <-chan scrapemate.Result) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.flush()
			return ctx.Err()
		case <-ticker.C:
			w.flush()
		case result, ok := <-in:
			if !ok {
				w.flush()
				return nil
			}

			w.processResult(result)
		}
	}
}

func (w *gsWriter) processResult(result scrapemate.Result) {
	var entry *gmaps.Entry
	switch v := result.Data.(type) {
	case *gmaps.Entry:
		entry = v
	case gmaps.Entry:
		entry = &v
	default:
		return
	}

	csvRow := entry.CsvRow()

	row := make([]interface{}, 0, len(csvRow)+1)
	for _, cell := range csvRow {
		row = append(row, cell)
	}

	row = append(row, "ok") // Scrape_Status

	w.mu.Lock()
	w.buffer = append(w.buffer, row)
	shouldFlush := len(w.buffer) >= w.maxSize
	w.mu.Unlock()

	if shouldFlush {
		w.flush()
	}
}

func (w *gsWriter) flush() {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return
	}

	dataCopy := make([][]interface{}, len(w.buffer))
	copy(dataCopy, w.buffer)
	w.buffer = w.buffer[:0]
	w.mu.Unlock()

	// Check if we need to overflow to a new tab
	if w.currentRows+int64(len(dataCopy)) > maxRowsPerSheet {
		if err := w.createNextSheet(); err != nil {
			log.Printf("Failed to create overflow sheet: %v", err)
			// Try to write to the current sheet anyway
		}
	}

	vr := &sheets.ValueRange{
		Values: dataCopy,
	}

	_, err := w.srv.Spreadsheets.Values.Append(w.spreadsheetID, w.currentSheet, vr).
		ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		log.Printf("Google Sheets Append Error: %v", err)
	} else {
		w.currentRows += int64(len(dataCopy))
		log.Printf("Appended %d rows to %q (total: %d)", len(dataCopy), w.currentSheet, w.currentRows)
	}
}

// createNextSheet adds a new tab and writes headers to it.
func (w *gsWriter) createNextSheet() error {
	w.sheetIndex++
	w.currentSheet = fmt.Sprintf("%s_%d", w.baseSheetName, w.sheetIndex)

	addReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: w.currentSheet,
					},
				},
			},
		},
	}

	_, err := w.srv.Spreadsheets.BatchUpdate(w.spreadsheetID, addReq).Do()
	if err != nil {
		return fmt.Errorf("failed to create sheet %q: %w", w.currentSheet, err)
	}

	log.Printf("Created overflow sheet: %q", w.currentSheet)

	return w.writeHeaders()
}
