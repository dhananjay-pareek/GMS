package leadsmanager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lead represents a Google Maps business lead stored in Supabase.
type Lead struct {
	PlaceID      string   `json:"place_id"`
	Title        string   `json:"title"`
	Category     string   `json:"category"`
	Categories   []string `json:"categories"`
	Address      string   `json:"address"`
	City         string   `json:"city"`
	State        string   `json:"state"`
	Country      string   `json:"country"`
	PostalCode   string   `json:"postal_code"`
	Phone        string   `json:"phone"`
	Emails       []string `json:"emails"`
	Website      string   `json:"website"`
	ReviewCount  int      `json:"review_count"`
	ReviewRating float64  `json:"review_rating"`
	Latitude     float64  `json:"latitude"`
	Longitude    float64  `json:"longitude"`
	GmapsLink    string   `json:"gmaps_link"`
	Cid          string   `json:"cid"`
	Status       string   `json:"status"`
	Description  string   `json:"description"`
	OwnerName    string   `json:"owner_name"`
	OwnerID      string   `json:"owner_id"`
	Thumbnail    string   `json:"thumbnail"`
	Timezone     string   `json:"timezone"`
	PriceRange   string   `json:"price_range"`
	PlusCode     string   `json:"plus_code"`

	// Enrichment fields
	IsEmailValid     bool     `json:"is_email_valid"`
	IsPhoneValid     bool     `json:"is_phone_valid"`
	HasSSL           *bool    `json:"has_ssl"`
	HasAnalytics     *bool    `json:"has_analytics"`
	HasFacebookPixel *bool    `json:"has_facebook_pixel"`
	HasH1            *bool    `json:"has_h1"`
	HasMetaDesc      *bool    `json:"has_meta_desc"`
	PageSpeedScore   *int     `json:"page_speed_score"`
	TechStack        []string `json:"tech_stack"`
	SocialLinks      string   `json:"social_links"`
	ServiceTags      []string `json:"service_tags"`
	GmbClaimed       bool     `json:"gmb_claimed"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LeadFilter holds query parameters for filtering leads.
type LeadFilter struct {
	Search    string  `json:"search"`
	City      string  `json:"city"`
	Category  string  `json:"category"`
	Tag       string  `json:"tag"`
	MinRating float64 `json:"min_rating"`
}

// DB wraps a pgxpool connection to Supabase PostgreSQL.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new database connection pool.
func NewDB(ctx context.Context, connStr string) (*DB, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	config.MaxConns = 10
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect to db: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close closes the database pool.
func (db *DB) Close() {
	db.pool.Close()
}

// UpsertLead inserts or updates a lead by place_id.
func (db *DB) UpsertLead(ctx context.Context, l *Lead) error {
	query := `
		INSERT INTO gmaps_leads (
			place_id, title, category, categories, address, city, state, country, postal_code,
			phone, emails, website, review_count, review_rating, latitude, longitude,
			gmaps_link, cid, status, description, owner_name, owner_id, thumbnail,
			timezone, price_range, plus_code,
			is_email_valid, is_phone_valid, has_ssl, has_analytics, has_facebook_pixel,
			has_h1, has_meta_desc, page_speed_score, tech_stack, social_links,
			service_tags, gmb_claimed
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22, $23,
			$24, $25, $26,
			$27, $28, $29, $30, $31,
			$32, $33, $34, $35, $36,
			$37, $38
		)
		ON CONFLICT (place_id) DO UPDATE SET
			title = EXCLUDED.title,
			category = EXCLUDED.category,
			categories = EXCLUDED.categories,
			address = EXCLUDED.address,
			city = EXCLUDED.city,
			state = EXCLUDED.state,
			country = EXCLUDED.country,
			postal_code = EXCLUDED.postal_code,
			phone = EXCLUDED.phone,
			emails = EXCLUDED.emails,
			website = EXCLUDED.website,
			review_count = EXCLUDED.review_count,
			review_rating = EXCLUDED.review_rating,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			gmaps_link = EXCLUDED.gmaps_link,
			cid = EXCLUDED.cid,
			status = EXCLUDED.status,
			description = EXCLUDED.description,
			owner_name = EXCLUDED.owner_name,
			owner_id = EXCLUDED.owner_id,
			thumbnail = EXCLUDED.thumbnail,
			timezone = EXCLUDED.timezone,
			price_range = EXCLUDED.price_range,
			plus_code = EXCLUDED.plus_code,
			is_email_valid = EXCLUDED.is_email_valid,
			is_phone_valid = EXCLUDED.is_phone_valid,
			has_ssl = EXCLUDED.has_ssl,
			has_analytics = EXCLUDED.has_analytics,
			has_facebook_pixel = EXCLUDED.has_facebook_pixel,
			has_h1 = EXCLUDED.has_h1,
			has_meta_desc = EXCLUDED.has_meta_desc,
			page_speed_score = EXCLUDED.page_speed_score,
			tech_stack = EXCLUDED.tech_stack,
			social_links = EXCLUDED.social_links,
			service_tags = EXCLUDED.service_tags,
			gmb_claimed = EXCLUDED.gmb_claimed
	`

	socialLinks := l.SocialLinks
	if socialLinks == "" {
		socialLinks = "{}"
	}

	_, err := db.pool.Exec(ctx, query,
		l.PlaceID, l.Title, l.Category, l.Categories, l.Address, l.City, l.State, l.Country, l.PostalCode,
		l.Phone, l.Emails, l.Website, l.ReviewCount, l.ReviewRating, l.Latitude, l.Longitude,
		l.GmapsLink, l.Cid, l.Status, l.Description, l.OwnerName, l.OwnerID, l.Thumbnail,
		l.Timezone, l.PriceRange, l.PlusCode,
		l.IsEmailValid, l.IsPhoneValid, l.HasSSL, l.HasAnalytics, l.HasFacebookPixel,
		l.HasH1, l.HasMetaDesc, l.PageSpeedScore, l.TechStack, socialLinks,
		l.ServiceTags, l.GmbClaimed,
	)

	return err
}

// UpsertLeads batch-inserts or updates multiple leads.
func (db *DB) UpsertLeads(ctx context.Context, leads []Lead) error {
	for i := range leads {
		if err := db.UpsertLead(ctx, &leads[i]); err != nil {
			return fmt.Errorf("upsert lead %s: %w", leads[i].PlaceID, err)
		}
	}
	return nil
}

// FetchLeads retrieves paginated and filtered leads.
func (db *DB) FetchLeads(ctx context.Context, filter LeadFilter, page, pageSize int) ([]Lead, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}

	var conditions []string
	var args []any
	argIdx := 1

	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(title ILIKE $%d OR address ILIKE $%d OR phone ILIKE $%d)",
			argIdx, argIdx, argIdx,
		))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	if filter.City != "" {
		conditions = append(conditions, fmt.Sprintf("city ILIKE $%d", argIdx))
		args = append(args, "%"+filter.City+"%")
		argIdx++
	}

	if filter.Category != "" {
		conditions = append(conditions, fmt.Sprintf("category ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Category+"%")
		argIdx++
	}

	if filter.Tag != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(service_tags)", argIdx))
		args = append(args, filter.Tag)
		argIdx++
	}

	if filter.MinRating > 0 {
		conditions = append(conditions, fmt.Sprintf("review_rating >= $%d", argIdx))
		args = append(args, filter.MinRating)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM gmaps_leads %s", whereClause)
	var total int
	if err := db.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count leads: %w", err)
	}

	// Fetch page
	offset := (page - 1) * pageSize
	dataQuery := fmt.Sprintf(`
		SELECT place_id, title, category, categories, address, city, state, country, postal_code,
			phone, emails, website, review_count, review_rating, latitude, longitude,
			gmaps_link, cid, status, description, owner_name, owner_id, thumbnail,
			timezone, price_range, plus_code,
			is_email_valid, is_phone_valid, has_ssl, has_analytics, has_facebook_pixel,
			has_h1, has_meta_desc, page_speed_score, tech_stack, social_links,
			service_tags, gmb_claimed, created_at, updated_at
		FROM gmaps_leads %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, pageSize, offset)

	rows, err := db.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query leads: %w", err)
	}
	defer rows.Close()

	leads, err := scanLeads(rows)
	if err != nil {
		return nil, 0, err
	}

	return leads, total, nil
}

// GetLead retrieves a single lead by place_id.
func (db *DB) GetLead(ctx context.Context, placeID string) (*Lead, error) {
	query := `
		SELECT place_id, title, category, categories, address, city, state, country, postal_code,
			phone, emails, website, review_count, review_rating, latitude, longitude,
			gmaps_link, cid, status, description, owner_name, owner_id, thumbnail,
			timezone, price_range, plus_code,
			is_email_valid, is_phone_valid, has_ssl, has_analytics, has_facebook_pixel,
			has_h1, has_meta_desc, page_speed_score, tech_stack, social_links,
			service_tags, gmb_claimed, created_at, updated_at
		FROM gmaps_leads
		WHERE place_id = $1
	`

	rows, err := db.pool.Query(ctx, query, placeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads, err := scanLeads(rows)
	if err != nil {
		return nil, err
	}

	if len(leads) == 0 {
		return nil, fmt.Errorf("lead not found: %s", placeID)
	}

	return &leads[0], nil
}

// GetCompetitors finds top-rated leads in the same city.
func (db *DB) GetCompetitors(ctx context.Context, city string, minRating float64) ([]Lead, error) {
	query := `
		SELECT place_id, title, category, categories, address, city, state, country, postal_code,
			phone, emails, website, review_count, review_rating, latitude, longitude,
			gmaps_link, cid, status, description, owner_name, owner_id, thumbnail,
			timezone, price_range, plus_code,
			is_email_valid, is_phone_valid, has_ssl, has_analytics, has_facebook_pixel,
			has_h1, has_meta_desc, page_speed_score, tech_stack, social_links,
			service_tags, gmb_claimed, created_at, updated_at
		FROM gmaps_leads
		WHERE city ILIKE $1 AND review_rating >= $2
		ORDER BY review_rating DESC, review_count DESC
		LIMIT 5
	`

	rows, err := db.pool.Query(ctx, query, city, minRating)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanLeads(rows)
}

func scanLeads(rows pgx.Rows) ([]Lead, error) {
	var leads []Lead

	for rows.Next() {
		var l Lead
		err := rows.Scan(
			&l.PlaceID, &l.Title, &l.Category, &l.Categories, &l.Address, &l.City, &l.State, &l.Country, &l.PostalCode,
			&l.Phone, &l.Emails, &l.Website, &l.ReviewCount, &l.ReviewRating, &l.Latitude, &l.Longitude,
			&l.GmapsLink, &l.Cid, &l.Status, &l.Description, &l.OwnerName, &l.OwnerID, &l.Thumbnail,
			&l.Timezone, &l.PriceRange, &l.PlusCode,
			&l.IsEmailValid, &l.IsPhoneValid, &l.HasSSL, &l.HasAnalytics, &l.HasFacebookPixel,
			&l.HasH1, &l.HasMetaDesc, &l.PageSpeedScore, &l.TechStack, &l.SocialLinks,
			&l.ServiceTags, &l.GmbClaimed, &l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan lead: %w", err)
		}

		leads = append(leads, l)
	}

	return leads, rows.Err()
}
