package leadsmanager

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// SupabaseDB reads leads from a Supabase-hosted PostgreSQL instance.
// It is read-only: all write methods are no-ops (writes happen via supabasewriter).
type SupabaseDB struct {
	pool *sql.DB
}

// NewSupabaseDB opens a connection to the Supabase Postgres instance.
func NewSupabaseDB(dsn string) (*SupabaseDB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("supabase_db: open: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("supabase_db: ping: %w", err)
	}

	log.Println("supabase_db: connected to Supabase Postgres")
	return &SupabaseDB{pool: db}, nil
}

func (s *SupabaseDB) Close() {
	if s != nil && s.pool != nil {
		_ = s.pool.Close()
	}
}

// supabaseSelectCols is the exact SELECT list that matches scanSupabaseLead's Scan() call.
// Order MUST match the Scan() argument order in scanSupabaseLead.
const supabaseSelectCols = `
  place_id, title, category, categories::text, address, city, state, country, postal_code,
  phone, emails::text, website, review_count, review_rating, latitude, longitude, gmaps_link,
  cid, status, description, service_tags::text,
  is_email_valid, is_phone_valid, gmb_claimed,
  has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score,
  tech_stack::text, social_links::text,
  owner_name, owner_id, thumbnail, timezone, price_range, plus_code,
  is_called, called_by, call_response, called_at,
  created_at, updated_at`

// FetchLeads queries public.gmaps_leads on Supabase with filtering and pagination.
func (s *SupabaseDB) FetchLeads(ctx context.Context, filter LeadFilter, page, pageSize int) ([]Lead, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}

	whereParts := []string{"1=1"}
	args := make([]any, 0, 8)
	paramIdx := 1

	placeholder := func() string {
		p := fmt.Sprintf("$%d", paramIdx)
		paramIdx++
		return p
	}

	if q := strings.TrimSpace(filter.Search); q != "" {
		like := "%" + q + "%"
		whereParts = append(whereParts,
			fmt.Sprintf("(LOWER(title) LIKE LOWER(%s) OR LOWER(address) LIKE LOWER(%s) OR LOWER(website) LIKE LOWER(%s))",
				placeholder(), placeholder(), placeholder()))
		args = append(args, like, like, like)
	}
	if c := strings.TrimSpace(filter.City); c != "" {
		whereParts = append(whereParts, fmt.Sprintf("LOWER(city) = LOWER(%s)", placeholder()))
		args = append(args, c)
	}
	if cat := strings.TrimSpace(filter.Category); cat != "" {
		whereParts = append(whereParts, fmt.Sprintf("LOWER(category) = LOWER(%s)", placeholder()))
		args = append(args, cat)
	}
	if t := strings.TrimSpace(filter.Tag); t != "" {
		whereParts = append(whereParts, fmt.Sprintf("%s = ANY(service_tags)", placeholder()))
		args = append(args, t)
	}
	if filter.MinRating > 0 {
		whereParts = append(whereParts, fmt.Sprintf("review_rating >= %s", placeholder()))
		args = append(args, filter.MinRating)
	}
	if filter.IsCalled != nil {
		whereParts = append(whereParts, fmt.Sprintf("is_called = %s", placeholder()))
		args = append(args, *filter.IsCalled)
	}

	whereSQL := strings.Join(whereParts, " AND ")

	var total int
	countQ := "SELECT COUNT(*) FROM public.gmaps_leads WHERE " + whereSQL
	if err := s.pool.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("supabase_db FetchLeads count: %w", err)
	}

	offset := (page - 1) * pageSize
	dataQ := `SELECT ` + supabaseSelectCols + `
FROM public.gmaps_leads
WHERE ` + whereSQL + `
ORDER BY updated_at DESC
LIMIT ` + fmt.Sprintf("$%d", paramIdx) + ` OFFSET ` + fmt.Sprintf("$%d", paramIdx+1)

	dataArgs := append(append([]any{}, args...), pageSize, offset)

	rows, err := s.pool.QueryContext(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("supabase_db FetchLeads query: %w", err)
	}
	defer rows.Close()

	leads := make([]Lead, 0, pageSize)
	for rows.Next() {
		lead, err := scanSupabaseLead(rows)
		if err != nil {
			log.Printf("supabase_db: scan error: %v", err)
			continue
		}
		leads = append(leads, lead)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	log.Printf("supabase_db: fetched %d/%d leads", len(leads), total)
	return leads, total, nil
}

// GetLead fetches a single lead from Supabase by place_id.
func (s *SupabaseDB) GetLead(ctx context.Context, placeID string) (*Lead, error) {
	q := `SELECT ` + supabaseSelectCols + `
FROM public.gmaps_leads WHERE place_id = $1 LIMIT 1`

	rows, err := s.pool.QueryContext(ctx, q, placeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("lead not found: %s", placeID)
	}
	lead, err := scanSupabaseLead(rows)
	if err != nil {
		return nil, err
	}
	return &lead, nil
}

// GetStats returns aggregate statistics from Supabase.
func (s *SupabaseDB) GetStats(ctx context.Context) (*DashboardStats, error) {
	const q = `
SELECT
  COUNT(*) AS total_leads,
  COALESCE(SUM(CASE WHEN website <> '' THEN 1 ELSE 0 END), 0::bigint) AS with_website,
  COALESCE(SUM(CASE WHEN cardinality(emails) > 0 THEN 1 ELSE 0 END), 0::bigint) AS with_email,
  COALESCE(AVG(CASE WHEN review_rating > 0 THEN review_rating END), 0.0) AS avg_rating,
  COALESCE(SUM(CASE WHEN cardinality(service_tags) > 2 THEN 1 ELSE 0 END), 0::bigint) AS flagged_count
FROM public.gmaps_leads`

	var stats DashboardStats
	err := s.pool.QueryRowContext(ctx, q).Scan(
		&stats.TotalLeads,
		&stats.WithWebsite,
		&stats.WithEmail,
		&stats.AvgRating,
		&stats.FlaggedCount,
	)
	if err != nil {
		return nil, fmt.Errorf("supabase_db GetStats: %w", err)
	}
	return &stats, nil
}

// GetCompetitors fetches top-rated leads by city from Supabase.
func (s *SupabaseDB) GetCompetitors(ctx context.Context, city string, minRating float64) ([]Lead, error) {
	q := `SELECT ` + supabaseSelectCols + `
FROM public.gmaps_leads
WHERE LOWER(city) = LOWER($1) AND review_rating >= $2
ORDER BY review_rating DESC, review_count DESC LIMIT 10`

	rows, err := s.pool.QueryContext(ctx, q, city, minRating)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []Lead
	for rows.Next() {
		lead, err := scanSupabaseLead(rows)
		if err != nil {
			continue
		}
		leads = append(leads, lead)
	}
	return leads, rows.Err()
}

// GetCompetitorsByCategory fetches competitors filtered by city and category from Supabase.
func (s *SupabaseDB) GetCompetitorsByCategory(ctx context.Context, city, category, excludePlaceID string, minRating float64) ([]Lead, error) {
	q := `SELECT ` + supabaseSelectCols + `
FROM public.gmaps_leads
WHERE LOWER(city) = LOWER($1)
  AND LOWER(category) = LOWER($2)
  AND place_id != $3
  AND review_rating >= $4
ORDER BY review_rating DESC, review_count DESC LIMIT 10`

	rows, err := s.pool.QueryContext(ctx, q, city, category, excludePlaceID, minRating)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []Lead
	for rows.Next() {
		lead, err := scanSupabaseLead(rows)
		if err != nil {
			continue
		}
		leads = append(leads, lead)
	}
	return leads, rows.Err()
}

// Write operations are no-ops — Supabase writes go through supabasewriter only.
func (s *SupabaseDB) UpsertLeads(_ context.Context, _ []Lead) error                 { return nil }
func (s *SupabaseDB) UpdateTechStack(_ context.Context, _ string, _ []string) error { return nil }
func (s *SupabaseDB) UpdatePageSpeedScore(_ context.Context, _ string, _ int) error { return nil }
func (s *SupabaseDB) UpdateEmails(_ context.Context, _ string, _ []string, _ bool) error {
	return nil
}

func (s *SupabaseDB) UpdateCallStatus(ctx context.Context, placeID, calledBy, response string) error {
	now := time.Now().UTC()
	_, err := s.pool.ExecContext(ctx, "UPDATE public.gmaps_leads SET is_called = true, called_by = $1, call_response = $2, called_at = $3, updated_at = $4 WHERE place_id = $5",
		calledBy, response, now, now, placeID)
	return err
}

// scanSupabaseLead scans a row that was selected with supabaseSelectCols.
// Column order MUST match supabaseSelectCols exactly.
func scanSupabaseLead(rows *sql.Rows) (Lead, error) {
	var (
		lead                                                       Lead
		categoriesArr, emailsArr, serviceTagsArr, techStackArr     string
		isEmailValid, isPhoneValid, gmbClaimed                     sql.NullBool
		hasSSL, hasAnalytics, hasFacebookPixel, hasH1, hasMetaDesc sql.NullBool
		pageSpeedScore                                             sql.NullInt64
		isCalled                                                   sql.NullBool
		calledAt                                                   sql.NullTime
		createdAt, updatedAt                                       time.Time
	)

	err := rows.Scan(
		// place_id, title, category, categories, address, city, state, country, postal_code
		&lead.PlaceID, &lead.Title, &lead.Category, &categoriesArr,
		&lead.Address, &lead.City, &lead.State, &lead.Country, &lead.PostalCode,
		// phone, emails, website, review_count, review_rating, latitude, longitude, gmaps_link
		&lead.Phone, &emailsArr, &lead.Website,
		&lead.ReviewCount, &lead.ReviewRating,
		&lead.Latitude, &lead.Longitude, &lead.GmapsLink,
		// cid, status, description, service_tags
		&lead.Cid, &lead.Status, &lead.Description, &serviceTagsArr,
		// is_email_valid, is_phone_valid, gmb_claimed
		&isEmailValid, &isPhoneValid, &gmbClaimed,
		// has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score
		&hasSSL, &hasAnalytics, &hasFacebookPixel, &hasH1, &hasMetaDesc, &pageSpeedScore,
		// tech_stack, social_links::text
		&techStackArr, &lead.SocialLinks,
		// owner_name, owner_id, thumbnail, timezone, price_range, plus_code
		&lead.OwnerName, &lead.OwnerID, &lead.Thumbnail, &lead.Timezone,
		&lead.PriceRange, &lead.PlusCode,
		// is_called, called_by, call_response, called_at
		&isCalled, &lead.CalledBy, &lead.CallResponse, &calledAt,
		// created_at, updated_at
		&createdAt, &updatedAt,
	)
	if err != nil {
		return Lead{}, err
	}

	lead.Categories = parsePgArray(categoriesArr)
	lead.Emails = parsePgArray(emailsArr)
	lead.ServiceTags = parsePgArray(serviceTagsArr)
	lead.TechStack = parsePgArray(techStackArr)

	if isEmailValid.Valid {
		lead.IsEmailValid = isEmailValid.Bool
	}
	if isPhoneValid.Valid {
		lead.IsPhoneValid = isPhoneValid.Bool
	}
	if gmbClaimed.Valid {
		lead.GmbClaimed = gmbClaimed.Bool
	}
	if hasSSL.Valid {
		v := hasSSL.Bool
		lead.HasSSL = &v
	}
	if hasAnalytics.Valid {
		v := hasAnalytics.Bool
		lead.HasAnalytics = &v
	}
	if hasFacebookPixel.Valid {
		v := hasFacebookPixel.Bool
		lead.HasFacebookPixel = &v
	}
	if hasH1.Valid {
		v := hasH1.Bool
		lead.HasH1 = &v
	}
	if hasMetaDesc.Valid {
		v := hasMetaDesc.Bool
		lead.HasMetaDesc = &v
	}
	if isCalled.Valid {
		lead.IsCalled = isCalled.Bool
	}
	if calledAt.Valid {
		v := calledAt.Time
		lead.CalledAt = &v
	}

	lead.CreatedAt = createdAt
	lead.UpdatedAt = updatedAt

	return lead, nil
}

// parsePgArray converts a Postgres text[] literal (e.g. {foo,bar}) to a Go slice.
func parsePgArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return []string{}
	}
	if len(raw) >= 2 && raw[0] == '{' && raw[len(raw)-1] == '}' {
		raw = raw[1 : len(raw)-1]
	}
	if raw == "" {
		return []string{}
	}
	var result []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch == '"' && !inQuote:
			inQuote = true
		case ch == '"' && inQuote:
			inQuote = false
		case ch == ',' && !inQuote:
			result = append(result, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}
