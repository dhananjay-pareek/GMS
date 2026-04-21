package leadsmanager

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Lead struct {
	PlaceID          string     `json:"place_id"`
	Title            string     `json:"title"`
	Category         string     `json:"category"`
	Categories       []string   `json:"categories"`
	Address          string     `json:"address"`
	City             string     `json:"city"`
	State            string     `json:"state"`
	Country          string     `json:"country"`
	PostalCode       string     `json:"postal_code"`
	Phone            string     `json:"phone"`
	Emails           []string   `json:"emails"`
	Website          string     `json:"website"`
	ReviewCount      int        `json:"review_count"`
	ReviewRating     float64    `json:"review_rating"`
	Latitude         float64    `json:"latitude"`
	Longitude        float64    `json:"longitude"`
	GmapsLink        string     `json:"gmaps_link"`
	Cid              string     `json:"cid"`
	Status           string     `json:"status"`
	Description      string     `json:"description"`
	ServiceTags      []string   `json:"service_tags"`
	IsEmailValid     bool       `json:"is_email_valid"`
	IsPhoneValid     bool       `json:"is_phone_valid"`
	GmbClaimed       bool       `json:"gmb_claimed"`
	HasSSL           *bool      `json:"has_ssl"`
	HasAnalytics     *bool      `json:"has_analytics"`
	HasFacebookPixel *bool      `json:"has_facebook_pixel"`
	HasH1            *bool      `json:"has_h1"`
	HasMetaDesc      *bool      `json:"has_meta_desc"`
	PageSpeedScore   *int       `json:"page_speed_score"`
	TechStack        []string   `json:"tech_stack"`
	SocialLinks      string     `json:"social_links"`
	OwnerName        string     `json:"owner_name"`
	OwnerID          string     `json:"owner_id"`
	Thumbnail        string     `json:"thumbnail"`
	Timezone         string     `json:"timezone"`
	PriceRange       string     `json:"price_range"`
	PlusCode         string     `json:"plus_code"`
	IsCalled         bool       `json:"is_called"`
	CalledBy         string     `json:"called_by"`
	CallResponse     string     `json:"call_response"`
	CalledAt         *time.Time `json:"called_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type LeadFilter struct {
	Search    string
	City      string
	Category  string
	Tag       string
	MinRating float64
	IsCalled  *bool
}

// LeadStore is the read/write interface implemented by *DB (SQLite),
// *SupabaseDB (Postgres), and *CombinedDB.
type LeadStore interface {
	FetchLeads(ctx context.Context, filter LeadFilter, page, pageSize int) ([]Lead, int, error)
	GetCities(ctx context.Context) ([]string, error)
	GetCategories(ctx context.Context) ([]string, error)
	GetLead(ctx context.Context, placeID string) (*Lead, error)
	GetStats(ctx context.Context) (*DashboardStats, error)
	GetCompetitors(ctx context.Context, city string, minRating float64) ([]Lead, error)
	GetCompetitorsByCategory(ctx context.Context, city, category, excludePlaceID string, minRating float64) ([]Lead, error)
	UpsertLeads(ctx context.Context, leads []Lead) error
	UpdateTechStack(ctx context.Context, placeID string, techs []string) error
	UpdatePageSpeedScore(ctx context.Context, placeID string, score int) error
	UpdateEmails(ctx context.Context, placeID string, emails []string, isValid bool) error
	UpdateCallStatus(ctx context.Context, placeID, calledBy, response string) error
	KeepAlive(ctx context.Context) error
	Close()
}

type DB struct {
	pool *sql.DB
}

const (
	EnvLeadsDBPath = "LEADS_DB_PATH"
)

func ResolveDBPath(dbPath string) string {
	if strings.TrimSpace(dbPath) != "" {
		return dbPath
	}

	if envPath := strings.TrimSpace(os.Getenv(EnvLeadsDBPath)); envPath != "" {
		return envPath
	}

	return filepath.Join("webdata", "leadsmanager.db")
}

func NewDB(ctx context.Context, dbPath string) (*DB, error) {
	dbPath = ResolveDBPath(dbPath)

	if err := os.MkdirAll(filepath.Dir(dbPath), os.ModePerm); err != nil {
		return nil, fmt.Errorf("create leads db directory: %w", err)
	}

	pool, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open leads db: %w", err)
	}

	if _, err := pool.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL; PRAGMA busy_timeout = 5000;"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("enable sqlite pragmas: %w", err)
	}

	db := &DB{pool: pool}
	if err := db.initSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() {
	if db != nil && db.pool != nil {
		_ = db.pool.Close()
	}
}

func (db *DB) KeepAlive(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return nil
	}
	return db.pool.PingContext(ctx)
}

func (db *DB) initSchema(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS gmaps_leads (
  place_id TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  categories_json TEXT NOT NULL DEFAULT '[]',
  address TEXT NOT NULL DEFAULT '',
  city TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  country TEXT NOT NULL DEFAULT '',
  postal_code TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  emails_json TEXT NOT NULL DEFAULT '[]',
  website TEXT NOT NULL DEFAULT '',
  review_count INTEGER NOT NULL DEFAULT 0,
  review_rating REAL NOT NULL DEFAULT 0,
  latitude REAL NOT NULL DEFAULT 0,
  longitude REAL NOT NULL DEFAULT 0,
  gmaps_link TEXT NOT NULL DEFAULT '',
  cid TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  service_tags_json TEXT NOT NULL DEFAULT '[]',
  is_email_valid INTEGER NOT NULL DEFAULT 0,
  is_phone_valid INTEGER NOT NULL DEFAULT 0,
  gmb_claimed INTEGER NOT NULL DEFAULT 0,
  has_ssl INTEGER NULL,
  has_analytics INTEGER NULL,
  has_facebook_pixel INTEGER NULL,
  has_h1 INTEGER NULL,
  has_meta_desc INTEGER NULL,
  page_speed_score INTEGER NULL,
  tech_stack_json TEXT NOT NULL DEFAULT '[]',
  social_links TEXT NOT NULL DEFAULT '{}',
  owner_name TEXT NOT NULL DEFAULT '',
  owner_id TEXT NOT NULL DEFAULT '',
  thumbnail TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT '',
  price_range TEXT NOT NULL DEFAULT '',
  plus_code TEXT NOT NULL DEFAULT '',
  is_called INTEGER NOT NULL DEFAULT 0,
  called_by TEXT NOT NULL DEFAULT '',
  call_response TEXT NOT NULL DEFAULT '',
  called_at TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gmaps_leads_city ON gmaps_leads(city);
CREATE INDEX IF NOT EXISTS idx_gmaps_leads_category ON gmaps_leads(category);
CREATE INDEX IF NOT EXISTS idx_gmaps_leads_rating ON gmaps_leads(review_rating);
`
	_, err := db.pool.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("init leads schema: %w", err)
	}

	// Migrate existing databases: add call tracking columns if they don't exist.
	// SQLite does not support IF NOT EXISTS in ALTER TABLE, so we ignore errors.
	migrations := []string{
		"ALTER TABLE gmaps_leads ADD COLUMN is_called INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE gmaps_leads ADD COLUMN called_by TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE gmaps_leads ADD COLUMN call_response TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE gmaps_leads ADD COLUMN called_at TEXT NULL",
	}
	for _, m := range migrations {
		_, _ = db.pool.ExecContext(ctx, m) // Ignore "duplicate column" errors
	}

	return nil
}

func (db *DB) BulkUpsertLeads(ctx context.Context, leads []Lead) (int, error) {
	if len(leads) == 0 {
		return 0, nil
	}

	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const query = `
INSERT INTO gmaps_leads (
  place_id, title, category, categories_json, address, city, state, country, postal_code,
  phone, emails_json, website, review_count, review_rating, latitude, longitude, gmaps_link,
  cid, status, description, service_tags_json, is_email_valid, is_phone_valid, gmb_claimed,
  has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score,
  tech_stack_json, social_links, owner_name, owner_id, thumbnail, timezone, price_range,
  plus_code, created_at, updated_at
) VALUES (
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(place_id) DO UPDATE SET
  title = excluded.title,
  category = excluded.category,
  categories_json = excluded.categories_json,
  address = excluded.address,
  city = excluded.city,
  state = excluded.state,
  country = excluded.country,
  postal_code = excluded.postal_code,
  phone = excluded.phone,
  emails_json = excluded.emails_json,
  website = excluded.website,
  review_count = excluded.review_count,
  review_rating = excluded.review_rating,
  latitude = excluded.latitude,
  longitude = excluded.longitude,
  gmaps_link = excluded.gmaps_link,
  cid = excluded.cid,
  status = excluded.status,
  description = excluded.description,
  service_tags_json = excluded.service_tags_json,
  is_email_valid = excluded.is_email_valid,
  is_phone_valid = excluded.is_phone_valid,
  gmb_claimed = excluded.gmb_claimed,
  has_ssl = excluded.has_ssl,
  has_analytics = excluded.has_analytics,
  has_facebook_pixel = excluded.has_facebook_pixel,
  has_h1 = excluded.has_h1,
  has_meta_desc = excluded.has_meta_desc,
  page_speed_score = excluded.page_speed_score,
  tech_stack_json = excluded.tech_stack_json,
  social_links = excluded.social_links,
  owner_name = excluded.owner_name,
  owner_id = excluded.owner_id,
  thumbnail = excluded.thumbnail,
  timezone = excluded.timezone,
  price_range = excluded.price_range,
  plus_code = excluded.plus_code,
  created_at = COALESCE(gmaps_leads.created_at, excluded.created_at),
  updated_at = excluded.updated_at;
`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	successCount := 0
	for _, lead := range leads {
		now := time.Now().UTC()
		if lead.CreatedAt.IsZero() {
			lead.CreatedAt = now
		}
		lead.UpdatedAt = now
		if lead.Categories == nil {
			lead.Categories = []string{}
		}
		if lead.Emails == nil {
			lead.Emails = []string{}
		}
		if lead.ServiceTags == nil {
			lead.ServiceTags = []string{}
		}
		if lead.TechStack == nil {
			lead.TechStack = []string{}
		}
		if lead.SocialLinks == "" {
			lead.SocialLinks = "{}"
		}

		_, err = stmt.ExecContext(ctx,
			lead.PlaceID,
			lead.Title,
			lead.Category,
			marshalStringSlice(lead.Categories),
			lead.Address,
			lead.City,
			lead.State,
			lead.Country,
			lead.PostalCode,
			lead.Phone,
			marshalStringSlice(lead.Emails),
			lead.Website,
			lead.ReviewCount,
			lead.ReviewRating,
			lead.Latitude,
			lead.Longitude,
			lead.GmapsLink,
			lead.Cid,
			lead.Status,
			lead.Description,
			marshalStringSlice(lead.ServiceTags),
			boolToInt(lead.IsEmailValid),
			boolToInt(lead.IsPhoneValid),
			boolToInt(lead.GmbClaimed),
			ptrBoolToSQL(lead.HasSSL),
			ptrBoolToSQL(lead.HasAnalytics),
			ptrBoolToSQL(lead.HasFacebookPixel),
			ptrBoolToSQL(lead.HasH1),
			ptrBoolToSQL(lead.HasMetaDesc),
			ptrIntToSQL(lead.PageSpeedScore),
			marshalStringSlice(lead.TechStack),
			lead.SocialLinks,
			lead.OwnerName,
			lead.OwnerID,
			lead.Thumbnail,
			lead.Timezone,
			lead.PriceRange,
			lead.PlusCode,
			lead.CreatedAt.Format(time.RFC3339),
			lead.UpdatedAt.Format(time.RFC3339),
		)
		if err == nil {
			successCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return successCount, err
	}

	return successCount, nil
}

func (db *DB) UpsertLeads(ctx context.Context, leads []Lead) error {
	_, err := db.BulkUpsertLeads(ctx, leads)
	return err
}

func (db *DB) FetchLeads(ctx context.Context, filter LeadFilter, page, pageSize int) ([]Lead, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}

	whereParts := []string{"1=1"}
	args := make([]any, 0, 8)

	if s := strings.TrimSpace(filter.Search); s != "" {
		whereParts = append(whereParts, "(LOWER(title) LIKE LOWER(?) OR LOWER(address) LIKE LOWER(?) OR LOWER(website) LIKE LOWER(?))")
		like := "%" + s + "%"
		args = append(args, like, like, like)
	}
	if s := strings.TrimSpace(filter.City); s != "" {
		whereParts = append(whereParts, "LOWER(city) = LOWER(?)")
		args = append(args, s)
	}
	if s := strings.TrimSpace(filter.Category); s != "" {
		whereParts = append(whereParts, "LOWER(category) = LOWER(?)")
		args = append(args, s)
	}
	if s := strings.TrimSpace(filter.Tag); s != "" {
		whereParts = append(whereParts, "INSTR(service_tags_json, ?) > 0")
		args = append(args, `"`+s+`"`)
	}
	if filter.IsCalled != nil {
		whereParts = append(whereParts, "is_called = ?")
		if *filter.IsCalled {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}

	whereSQL := strings.Join(whereParts, " AND ")

	countQuery := "SELECT COUNT(*) FROM gmaps_leads WHERE " + whereSQL
	var total int
	if err := db.pool.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	dataQuery := `
SELECT
  place_id, title, category, categories_json, address, city, state, country, postal_code,
  phone, emails_json, website, review_count, review_rating, latitude, longitude, gmaps_link,
  cid, status, description, service_tags_json, is_email_valid, is_phone_valid, gmb_claimed,
  has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score,
  tech_stack_json, social_links, owner_name, owner_id, thumbnail, timezone, price_range,
  plus_code, is_called, called_by, call_response, called_at, created_at, updated_at
FROM gmaps_leads
WHERE ` + whereSQL + `
ORDER BY updated_at DESC
LIMIT ? OFFSET ?`
	dataArgs := append(append([]any{}, args...), pageSize, offset)

	rows, err := db.pool.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	leads := make([]Lead, 0, pageSize)
	for rows.Next() {
		lead, err := scanLead(rows)
		if err != nil {
			return nil, 0, err
		}
		leads = append(leads, lead)
	}

	return leads, total, rows.Err()
}

func (db *DB) GetCities(ctx context.Context) ([]string, error) {
	const query = `
SELECT DISTINCT TRIM(city) AS city
FROM gmaps_leads
WHERE TRIM(city) <> ''
ORDER BY LOWER(TRIM(city)) ASC`

	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cities := make([]string, 0)
	for rows.Next() {
		var city string
		if err := rows.Scan(&city); err != nil {
			return nil, err
		}
		cities = append(cities, city)
	}

	return cities, rows.Err()
}

func (db *DB) GetCategories(ctx context.Context) ([]string, error) {
	const query = `
SELECT DISTINCT TRIM(category) AS category
FROM gmaps_leads
WHERE TRIM(category) <> ''
ORDER BY LOWER(TRIM(category)) ASC`

	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]string, 0)
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}

	return categories, rows.Err()
}

func (db *DB) GetLead(ctx context.Context, placeID string) (*Lead, error) {
	const query = `
SELECT
  place_id, title, category, categories_json, address, city, state, country, postal_code,
  phone, emails_json, website, review_count, review_rating, latitude, longitude, gmaps_link,
  cid, status, description, service_tags_json, is_email_valid, is_phone_valid, gmb_claimed,
  has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score,
  tech_stack_json, social_links, owner_name, owner_id, thumbnail, timezone, price_range,
  plus_code, is_called, called_by, call_response, called_at, created_at, updated_at
FROM gmaps_leads
WHERE place_id = ? LIMIT 1`

	rows, err := db.pool.QueryContext(ctx, query, placeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("lead not found: %s", placeID)
	}

	lead, err := scanLead(rows)
	if err != nil {
		return nil, err
	}
	return &lead, nil
}

func (db *DB) GetCompetitors(ctx context.Context, city string, minRating float64) ([]Lead, error) {
	const query = `
SELECT
  place_id, title, category, categories_json, address, city, state, country, postal_code,
  phone, emails_json, website, review_count, review_rating, latitude, longitude, gmaps_link,
  cid, status, description, service_tags_json, is_email_valid, is_phone_valid, gmb_claimed,
  has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score,
  tech_stack_json, social_links, owner_name, owner_id, thumbnail, timezone, price_range,
  plus_code, is_called, called_by, call_response, called_at, created_at, updated_at
FROM gmaps_leads
WHERE LOWER(city) = LOWER(?) AND review_rating >= ?
ORDER BY review_rating DESC, review_count DESC
LIMIT 10`

	rows, err := db.pool.QueryContext(ctx, query, city, minRating)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []Lead
	for rows.Next() {
		lead, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		leads = append(leads, lead)
	}
	return leads, rows.Err()
}

func (db *DB) GetCompetitorsByCategory(ctx context.Context, city, category, excludePlaceID string, minRating float64) ([]Lead, error) {
	const query = `
SELECT
  place_id, title, category, categories_json, address, city, state, country, postal_code,
  phone, emails_json, website, review_count, review_rating, latitude, longitude, gmaps_link,
  cid, status, description, service_tags_json, is_email_valid, is_phone_valid, gmb_claimed,
  has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score,
  tech_stack_json, social_links, owner_name, owner_id, thumbnail, timezone, price_range,
  plus_code, is_called, called_by, call_response, called_at, created_at, updated_at
FROM gmaps_leads
WHERE LOWER(city) = LOWER(?)
  AND LOWER(category) = LOWER(?)
  AND place_id != ?
  AND review_rating >= ?
ORDER BY review_rating DESC, review_count DESC
LIMIT 10`

	rows, err := db.pool.QueryContext(ctx, query, city, category, excludePlaceID, minRating)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []Lead
	for rows.Next() {
		lead, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		leads = append(leads, lead)
	}
	return leads, rows.Err()
}

func (db *DB) UpdateTechStack(ctx context.Context, placeID string, techs []string) error {
	_, err := db.pool.ExecContext(ctx, "UPDATE gmaps_leads SET tech_stack_json = ?, updated_at = ? WHERE place_id = ?", marshalStringSlice(techs), time.Now().UTC().Format(time.RFC3339), placeID)
	return err
}

func (db *DB) UpdatePageSpeedScore(ctx context.Context, placeID string, score int) error {
	_, err := db.pool.ExecContext(ctx, "UPDATE gmaps_leads SET page_speed_score = ?, updated_at = ? WHERE place_id = ?", score, time.Now().UTC().Format(time.RFC3339), placeID)
	return err
}

func (db *DB) UpdateEmails(ctx context.Context, placeID string, emails []string, isValid bool) error {
	_, err := db.pool.ExecContext(ctx, "UPDATE gmaps_leads SET emails_json = ?, is_email_valid = ?, updated_at = ? WHERE place_id = ?", marshalStringSlice(emails), boolToInt(isValid), time.Now().UTC().Format(time.RFC3339), placeID)
	return err
}

func (db *DB) UpdateCallStatus(ctx context.Context, placeID, calledBy, response string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.pool.ExecContext(ctx, "UPDATE gmaps_leads SET is_called = 1, called_by = ?, call_response = ?, called_at = ?, updated_at = ? WHERE place_id = ?",
		calledBy, response, now, now, placeID)
	return err
}

func (db *DB) GetStats(ctx context.Context) (*DashboardStats, error) {
	const query = `
SELECT
  COUNT(*) AS total_leads,
  COALESCE(SUM(CASE WHEN website <> '' THEN 1 ELSE 0 END), 0) AS with_website,
  COALESCE(SUM(CASE WHEN json_array_length(emails_json) > 0 THEN 1 ELSE 0 END), 0) AS with_email,
  COALESCE(AVG(CASE WHEN review_rating > 0 THEN review_rating END), 0) AS avg_rating,
  COALESCE(SUM(CASE WHEN json_array_length(service_tags_json) > 2 THEN 1 ELSE 0 END), 0) AS flagged_count
FROM gmaps_leads`

	var stats DashboardStats
	err := db.pool.QueryRowContext(ctx, query).Scan(
		&stats.TotalLeads,
		&stats.WithWebsite,
		&stats.WithEmail,
		&stats.AvgRating,
		&stats.FlaggedCount,
	)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func scanLead(rows *sql.Rows) (Lead, error) {
	var (
		lead                                                       Lead
		categoriesJSON, emailsJSON, serviceTagsJSON, techStackJSON string
		createdAt, updatedAt                                       string
		isEmailValid, isPhoneValid, gmbClaimed                     int
		hasSSL, hasAnalytics, hasFacebookPixel, hasH1, hasMetaDesc sql.NullInt64
		pageSpeedScore                                             sql.NullInt64
		isCalled                                                   int
		calledAt                                                   sql.NullString
	)

	err := rows.Scan(
		&lead.PlaceID, &lead.Title, &lead.Category, &categoriesJSON, &lead.Address, &lead.City, &lead.State, &lead.Country, &lead.PostalCode,
		&lead.Phone, &emailsJSON, &lead.Website, &lead.ReviewCount, &lead.ReviewRating, &lead.Latitude, &lead.Longitude, &lead.GmapsLink,
		&lead.Cid, &lead.Status, &lead.Description, &serviceTagsJSON, &isEmailValid, &isPhoneValid, &gmbClaimed,
		&hasSSL, &hasAnalytics, &hasFacebookPixel, &hasH1, &hasMetaDesc, &pageSpeedScore,
		&techStackJSON, &lead.SocialLinks, &lead.OwnerName, &lead.OwnerID, &lead.Thumbnail, &lead.Timezone, &lead.PriceRange,
		&lead.PlusCode, &isCalled, &lead.CalledBy, &lead.CallResponse, &calledAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return Lead{}, err
	}

	lead.Categories = unmarshalStringSlice(categoriesJSON)
	lead.Emails = unmarshalStringSlice(emailsJSON)
	lead.ServiceTags = unmarshalStringSlice(serviceTagsJSON)
	lead.TechStack = unmarshalStringSlice(techStackJSON)
	lead.IsEmailValid = isEmailValid == 1
	lead.IsPhoneValid = isPhoneValid == 1
	lead.GmbClaimed = gmbClaimed == 1
	lead.HasSSL = nullIntToBoolPtr(hasSSL)
	lead.HasAnalytics = nullIntToBoolPtr(hasAnalytics)
	lead.HasFacebookPixel = nullIntToBoolPtr(hasFacebookPixel)
	lead.HasH1 = nullIntToBoolPtr(hasH1)
	lead.HasMetaDesc = nullIntToBoolPtr(hasMetaDesc)
	if pageSpeedScore.Valid {
		v := int(pageSpeedScore.Int64)
		lead.PageSpeedScore = &v
	}

	lead.Timezone = strings.TrimSpace(lead.Timezone)

	lead.IsCalled = isCalled == 1
	lead.CalledBy = strings.TrimSpace(lead.CalledBy)
	lead.CallResponse = strings.TrimSpace(lead.CallResponse)
	if calledAt.Valid {
		t, _ := time.Parse(time.RFC3339, calledAt.String)
		lead.CalledAt = &t
	}

	lead.CreatedAt = parseRFC3339(createdAt)
	lead.UpdatedAt = parseRFC3339(updatedAt)

	return lead, nil
}

func parseRFC3339(v string) time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}

func nullIntToBoolPtr(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Int64 == 1
	return &b
}

func marshalStringSlice(v []string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func ptrBoolToSQL(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}

func ptrIntToSQL(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "not found")
}
