package supabasewriter

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
	"github.com/gosom/scrapemate"
)

type writer struct {
	db *sql.DB
}

// New creates a ResultWriter that persists scraped entries into a
// Supabase-hosted PostgreSQL table (public.gmaps_leads).
func New(dsn string) (scrapemate.ResultWriter, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("supabasewriter: open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("supabasewriter: ping db: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	return &writer{db: db}, nil
}

func (w *writer) Run(ctx context.Context, in <-chan scrapemate.Result) error {
	const batchSize = 50
	var buffer []leadsmanager.Lead

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var processData func(any)
	processData = func(data any) {
		switch v := data.(type) {
		case *gmaps.Entry:
			if v != nil && v.PlaceID != "" {
				buffer = append(buffer, leadsmanager.ProcessEntry(*v))
			}
		case gmaps.Entry:
			if v.PlaceID != "" {
				buffer = append(buffer, leadsmanager.ProcessEntry(v))
			}
		case []*gmaps.Entry:
			for _, e := range v {
				if e != nil && e.PlaceID != "" {
					buffer = append(buffer, leadsmanager.ProcessEntry(*e))
				}
			}
		case []gmaps.Entry:
			for _, e := range v {
				if e.PlaceID != "" {
					buffer = append(buffer, leadsmanager.ProcessEntry(e))
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
				// We use a background context here because ctx is already done
				ctxFlush, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				w.saveBatch(ctxFlush, buffer)
				cancel()
			}
			w.db.Close()
			return ctx.Err()
		}
	}
}

func (w *writer) saveBatch(ctx context.Context, leads []leadsmanager.Lead) error {
	if len(leads) == 0 {
		return nil
	}

	const numCols = 40
	now := time.Now().UTC()

	var placeholders []string
	var args []any

	for i, lead := range leads {
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

		rowPlaceholders := make([]string, numCols)
		for j := 0; j < numCols; j++ {
			rowPlaceholders[j] = fmt.Sprintf("$%d", i*numCols+j+1)
		}
		placeholders = append(placeholders, "("+strings.Join(rowPlaceholders, ",")+")")

		args = append(args,
			lead.PlaceID, lead.Title, lead.Category, pqStringArray(lead.Categories),
			lead.Address, lead.City, lead.State, lead.Country, lead.PostalCode,
			lead.Phone, pqStringArray(lead.Emails), lead.Website,
			lead.ReviewCount, lead.ReviewRating, lead.Latitude, lead.Longitude,
			lead.GmapsLink, lead.Cid, lead.Status, lead.Description,
			pqStringArray(lead.ServiceTags), lead.IsEmailValid, lead.IsPhoneValid,
			lead.GmbClaimed, lead.HasSSL, lead.HasAnalytics, lead.HasFacebookPixel,
			lead.HasH1, lead.HasMetaDesc, lead.PageSpeedScore,
			pqStringArray(lead.TechStack), marshalJSON(lead.SocialLinks),
			lead.OwnerName, lead.OwnerID, lead.Thumbnail, lead.Timezone,
			lead.PriceRange, lead.PlusCode, lead.CreatedAt, lead.UpdatedAt,
		)
	}

	query := fmt.Sprintf(`
INSERT INTO public.gmaps_leads (
  place_id, title, category, categories, address, city, state, country, postal_code,
  phone, emails, website, review_count, review_rating, latitude, longitude, gmaps_link,
  cid, status, description, service_tags, is_email_valid, is_phone_valid, gmb_claimed,
  has_ssl, has_analytics, has_facebook_pixel, has_h1, has_meta_desc, page_speed_score,
  tech_stack, social_links, owner_name, owner_id, thumbnail, timezone, price_range,
  plus_code, created_at, updated_at
) VALUES %s
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
  service_tags = EXCLUDED.service_tags,
  is_email_valid = EXCLUDED.is_email_valid,
  is_phone_valid = EXCLUDED.is_phone_valid,
  gmb_claimed = EXCLUDED.gmb_claimed,
  has_ssl = EXCLUDED.has_ssl,
  has_analytics = EXCLUDED.has_analytics,
  has_facebook_pixel = EXCLUDED.has_facebook_pixel,
  has_h1 = EXCLUDED.has_h1,
  has_meta_desc = EXCLUDED.has_meta_desc,
  page_speed_score = EXCLUDED.page_speed_score,
  tech_stack = EXCLUDED.tech_stack,
  social_links = EXCLUDED.social_links,
  owner_name = EXCLUDED.owner_name,
  owner_id = EXCLUDED.owner_id,
  thumbnail = EXCLUDED.thumbnail,
  timezone = EXCLUDED.timezone,
  price_range = EXCLUDED.price_range,
  plus_code = EXCLUDED.plus_code,
  updated_at = EXCLUDED.updated_at`, strings.Join(placeholders, ","))

	err := w.save(ctx, query, args...)
	if err == context.Canceled || err == context.DeadlineExceeded {
		// If original context was cancelled mid-save, try one last time with a fresh background context
		ctxFlush, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = w.save(ctxFlush, query, args...)
	}

	if err != nil {
		return fmt.Errorf("supabase batch upsert error: %w", err)
	}

	log.Printf("supabase: batch saved %d leads", len(leads))
	return nil
}

func (w *writer) save(ctx context.Context, query string, args ...any) error {
	_, err := w.db.ExecContext(ctx, query, args...)
	return err
}

// pqStringArray formats a Go string slice as a PostgreSQL text[] literal.
func pqStringArray(ss []string) string {
	if len(ss) == 0 {
		return "{}"
	}
	escaped := make([]string, len(ss))
	for i, s := range ss {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		escaped[i] = `"` + s + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}"
}

// marshalJSON returns valid JSON; if input is already valid JSON return as-is,
// otherwise wrap in a JSON string.
func marshalJSON(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "{}"
	}
	if json.Valid([]byte(v)) {
		return v
	}
	b, _ := json.Marshal(v)
	return string(b)
}
