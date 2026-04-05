package leadsmanager

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const maxRowsPerSheet = 180000

// SheetsSyncer handles syncing leads to Google Sheets as a backup.
type SheetsSyncer struct {
	spreadsheetID string
	baseSheetName string
	srv           *sheets.Service

	mu           sync.Mutex
	buffer       [][]interface{}
	maxSize      int
	currentSheet string
	currentRows  int64
	sheetIndex   int
	enabled      bool
}

// NewSheetsSyncer creates a new Google Sheets syncer if credentials are available.
// Returns nil (not an error) if credentials are not configured - this makes it optional.
func NewSheetsSyncer() *SheetsSyncer {
	spreadsheetID := os.Getenv("GOOGLE_SHEET_ID")
	if spreadsheetID == "" {
		log.Println("Google Sheets sync disabled: GOOGLE_SHEET_ID not set")
		return nil
	}

	credsJSON := os.Getenv("GOOGLE_CREDENTIALS_JSON")
	if credsJSON == "" {
		log.Println("Google Sheets sync disabled: GOOGLE_CREDENTIALS_JSON not set")
		return nil
	}

	ctx := context.Background()
	config, err := google.JWTConfigFromJSON([]byte(credsJSON), sheets.SpreadsheetsScope)
	if err != nil {
		log.Printf("Google Sheets sync disabled: invalid credentials: %v", err)
		return nil
	}

	client := config.Client(ctx)
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Printf("Google Sheets sync disabled: failed to create service: %v", err)
		return nil
	}

	sheetName := "Leads"

	syncer := &SheetsSyncer{
		spreadsheetID: spreadsheetID,
		baseSheetName: sheetName,
		srv:           srv,
		maxSize:       50, // Sync every 50 rows
		currentSheet:  sheetName,
		sheetIndex:    1,
		enabled:       true,
	}

	// Initialize sheet (check existing rows, write headers if needed)
	if err := syncer.initSheet(); err != nil {
		log.Printf("Google Sheets init warning: %v - will retry on first sync", err)
	}

	log.Printf("Google Sheets sync enabled for spreadsheet: %s", spreadsheetID)
	return syncer
}

// IsEnabled returns true if the syncer is properly configured.
func (s *SheetsSyncer) IsEnabled() bool {
	return s != nil && s.enabled
}

// initSheet checks the current sheet's row count and writes headers if empty.
func (s *SheetsSyncer) initSheet() error {
	resp, err := s.srv.Spreadsheets.Values.Get(s.spreadsheetID, s.currentSheet).Do()
	if err != nil {
		// Sheet might not exist yet; try to create it
		log.Printf("Sheet %q doesn't exist, creating it...", s.currentSheet)
		if createErr := s.createSheet(s.currentSheet); createErr != nil {
			return fmt.Errorf("create sheet %q: %w", s.currentSheet, createErr)
		}
		s.currentRows = 0
		return s.writeHeaders()
	}

	s.currentRows = int64(len(resp.Values))

	if s.currentRows == 0 {
		return s.writeHeaders()
	}

	log.Printf("Sheet %q already has %d rows", s.currentSheet, s.currentRows)
	return nil
}

// createSheet creates a new sheet tab.
func (s *SheetsSyncer) createSheet(name string) error {
	addReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: name,
					},
				},
			},
		},
	}

	_, err := s.srv.Spreadsheets.BatchUpdate(s.spreadsheetID, addReq).Do()
	return err
}

// writeHeaders writes the column headers to the current sheet.
func (s *SheetsSyncer) writeHeaders() error {
	headers := []interface{}{
		"Place ID", "Title", "Category", "Address", "City", "State", "Country",
		"Phone", "Emails", "Website", "Review Count", "Review Rating",
		"Latitude", "Longitude", "Google Maps Link", "CID", "Status",
		"Description", "Service Tags", "Created At", "Synced At",
	}

	vr := &sheets.ValueRange{
		Values: [][]interface{}{headers},
	}

	_, err := s.srv.Spreadsheets.Values.Append(s.spreadsheetID, s.currentSheet, vr).
		ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("write headers to %q: %w", s.currentSheet, err)
	}

	s.currentRows = 1
	log.Printf("Wrote headers to sheet %q", s.currentSheet)
	return nil
}

// SyncLeads syncs a batch of leads to Google Sheets.
func (s *SheetsSyncer) SyncLeads(ctx context.Context, leads []Lead) error {
	if !s.IsEnabled() {
		return nil
	}

	s.mu.Lock()
	for _, lead := range leads {
		row := s.leadToRow(lead)
		s.buffer = append(s.buffer, row)
	}
	shouldFlush := len(s.buffer) >= s.maxSize
	s.mu.Unlock()

	if shouldFlush {
		return s.flush()
	}
	return nil
}

// leadToRow converts a Lead to a spreadsheet row.
func (s *SheetsSyncer) leadToRow(lead Lead) []interface{} {
	emails := strings.Join(lead.Emails, ", ")
	tags := strings.Join(lead.ServiceTags, ", ")

	return []interface{}{
		lead.PlaceID,
		lead.Title,
		lead.Category,
		lead.Address,
		lead.City,
		lead.State,
		lead.Country,
		lead.Phone,
		emails,
		lead.Website,
		lead.ReviewCount,
		lead.ReviewRating,
		lead.Latitude,
		lead.Longitude,
		lead.GmapsLink,
		lead.Cid,
		lead.Status,
		lead.Description,
		tags,
		lead.CreatedAt.Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	}
}

// flush writes buffered rows to Google Sheets.
func (s *SheetsSyncer) flush() error {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return nil
	}

	dataCopy := make([][]interface{}, len(s.buffer))
	copy(dataCopy, s.buffer)
	s.buffer = s.buffer[:0]
	s.mu.Unlock()

	// Check if we need to overflow to a new sheet tab
	if s.currentRows+int64(len(dataCopy)) > maxRowsPerSheet {
		if err := s.createNextSheet(); err != nil {
			log.Printf("Failed to create overflow sheet: %v", err)
			// Try to write to current sheet anyway
		}
	}

	vr := &sheets.ValueRange{
		Values: dataCopy,
	}

	_, err := s.srv.Spreadsheets.Values.Append(s.spreadsheetID, s.currentSheet, vr).
		ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		log.Printf("Google Sheets append error: %v", err)
		return err
	}

	s.currentRows += int64(len(dataCopy))
	log.Printf("Synced %d leads to Google Sheets %q (total: %d)", len(dataCopy), s.currentSheet, s.currentRows)
	return nil
}

// createNextSheet creates a new overflow sheet tab.
func (s *SheetsSyncer) createNextSheet() error {
	s.sheetIndex++
	s.currentSheet = fmt.Sprintf("%s_%d", s.baseSheetName, s.sheetIndex)

	if err := s.createSheet(s.currentSheet); err != nil {
		return err
	}

	log.Printf("Created overflow sheet: %q", s.currentSheet)
	s.currentRows = 0
	return s.writeHeaders()
}

// Flush forces any buffered data to be written to Google Sheets.
func (s *SheetsSyncer) Flush() error {
	if !s.IsEnabled() {
		return nil
	}
	return s.flush()
}
