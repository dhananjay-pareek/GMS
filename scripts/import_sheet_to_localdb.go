package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
)

var (
	phoneRegex = regexp.MustCompile(`^[\+]?[0-9\s\-\(\)]{7,20}$`)
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	sciNumRe   = regexp.MustCompile(`^[+-]?\d+(?:\.\d+)?[eE][+-]?\d+$`)
)

type csvLead struct {
	PlaceID      string
	Title        string
	Category     string
	Address      string
	City         string
	State        string
	Country      string
	Phone        string
	Emails       string
	Website      string
	ReviewCount  string
	ReviewRating string
	Latitude     string
	Longitude    string
	GmapsLink    string
	Cid          string
	Status       string
	Description  string
	ServiceTags  string
	CreatedAt    string
	SyncedAt     string
}

func main() {
	var (
		csvPath string
		dbPath  string
	)

	flag.StringVar(&csvPath, "csv", "Master Leads - Leads.csv", "path to exported CSV file")
	flag.StringVar(&dbPath, "db", "webdata\\leadsmanager.db", "path to local leads SQLite database")
	flag.Parse()

	if strings.TrimSpace(csvPath) == "" {
		exitErr("csv path is required")
	}

	ctx := context.Background()
	db, err := leadsmanager.NewDB(ctx, dbPath)
	if err != nil {
		exitErr(fmt.Sprintf("open local db: %v", err))
	}
	defer db.Close()

	leads, skipped, err := readCSV(csvPath)
	if err != nil {
		exitErr(fmt.Sprintf("read csv: %v", err))
	}

	const batchSize = 200
	imported := 0

	for start := 0; start < len(leads); start += batchSize {
		end := min(start+batchSize, len(leads))
		n, upsertErr := db.BulkUpsertLeads(ctx, leads[start:end])
		if upsertErr != nil {
			exitErr(fmt.Sprintf("bulk upsert rows %d..%d: %v", start, end, upsertErr))
		}
		imported += n
	}

	fmt.Printf("Imported %d leads into %s (skipped %d rows with empty Place ID)\n", imported, dbPath, skipped)
}

func readCSV(path string) ([]leadsmanager.Lead, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, 0, err
	}

	index := make(map[string]int, len(header))
	for i, h := range header {
		index[strings.TrimSpace(h)] = i
	}

	required := []string{
		"Place ID", "Title", "Category", "Address", "City", "State", "Country", "Phone", "Emails",
		"Website", "Review Count", "Review Rating", "Latitude", "Longitude", "Google Maps Link",
		"CID", "Status", "Description", "Service Tags", "Created At", "Synced At",
	}
	for _, col := range required {
		if _, ok := index[col]; !ok {
			return nil, 0, fmt.Errorf("missing required column: %s", col)
		}
	}

	var (
		leads   []leadsmanager.Lead
		skipped int
	)

	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, 0, readErr
		}

		entry := csvLead{
			PlaceID:      cell(row, index["Place ID"]),
			Title:        cell(row, index["Title"]),
			Category:     cell(row, index["Category"]),
			Address:      cell(row, index["Address"]),
			City:         cell(row, index["City"]),
			State:        cell(row, index["State"]),
			Country:      cell(row, index["Country"]),
			Phone:        cell(row, index["Phone"]),
			Emails:       cell(row, index["Emails"]),
			Website:      cell(row, index["Website"]),
			ReviewCount:  cell(row, index["Review Count"]),
			ReviewRating: cell(row, index["Review Rating"]),
			Latitude:     cell(row, index["Latitude"]),
			Longitude:    cell(row, index["Longitude"]),
			GmapsLink:    cell(row, index["Google Maps Link"]),
			Cid:          cell(row, index["CID"]),
			Status:       cell(row, index["Status"]),
			Description:  cell(row, index["Description"]),
			ServiceTags:  cell(row, index["Service Tags"]),
			CreatedAt:    cell(row, index["Created At"]),
			SyncedAt:     cell(row, index["Synced At"]),
		}

		if strings.TrimSpace(entry.PlaceID) == "" {
			skipped++
			continue
		}

		phone := normalizePhone(entry.Phone)
		isPhoneValid := validatePhone(phone)
		emails := splitCSVList(entry.Emails)
		isEmailValid := validateEmails(emails)
		serviceTags := normalizeServiceTags(splitCSVList(entry.ServiceTags), isPhoneValid)

		leads = append(leads, leadsmanager.Lead{
			PlaceID:      strings.TrimSpace(entry.PlaceID),
			Title:        strings.TrimSpace(entry.Title),
			Category:     strings.TrimSpace(entry.Category),
			Address:      strings.TrimSpace(entry.Address),
			City:         strings.TrimSpace(entry.City),
			State:        strings.TrimSpace(entry.State),
			Country:      strings.TrimSpace(entry.Country),
			Phone:        phone,
			Emails:       emails,
			Website:      strings.TrimSpace(entry.Website),
			ReviewCount:  atoi(entry.ReviewCount),
			ReviewRating: atof(entry.ReviewRating),
			Latitude:     atof(entry.Latitude),
			Longitude:    atof(entry.Longitude),
			GmapsLink:    strings.TrimSpace(entry.GmapsLink),
			Cid:          strings.TrimSpace(entry.Cid),
			Status:       strings.TrimSpace(entry.Status),
			Description:  strings.TrimSpace(entry.Description),
			ServiceTags:  serviceTags,
			IsPhoneValid: isPhoneValid,
			IsEmailValid: isEmailValid,
			CreatedAt:    parseTime(entry.CreatedAt),
			UpdatedAt:    parseTime(entry.SyncedAt),
		})
	}

	return leads, skipped, nil
}

func splitCSVList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func normalizePhone(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Trim(raw, "\"'")
	raw = strings.TrimPrefix(raw, "=")
	raw = strings.Trim(raw, "\"'")
	raw = strings.ReplaceAll(raw, "\u00A0", " ")
	raw = strings.TrimSpace(raw)

	if sciNumRe.MatchString(strings.ReplaceAll(raw, ",", "")) {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", ""), 64); err == nil {
			raw = strconv.FormatFloat(f, 'f', 0, 64)
		}
	}

	sign := ""
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "+") {
		sign = "+"
	}

	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	if digits == "" {
		return ""
	}

	return sign + digits
}

func validatePhone(phone string) bool {
	if phone == "" {
		return false
	}
	digits := 0
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= 7 && phoneRegex.MatchString(phone)
}

func validateEmails(emails []string) bool {
	for _, email := range emails {
		if emailRegex.MatchString(strings.TrimSpace(email)) {
			return true
		}
	}
	return false
}

func normalizeServiceTags(tags []string, isPhoneValid bool) []string {
	filtered := make([]string, 0, len(tags))
	hasNoPhone := false
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), "No Phone") {
			hasNoPhone = true
			if isPhoneValid {
				continue
			}
		}
		filtered = append(filtered, tag)
	}

	if !isPhoneValid && !hasNoPhone {
		filtered = append(filtered, "No Phone")
	}

	return filtered
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

func atoi(raw string) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return v
}

func atof(raw string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return v
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func exitErr(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
