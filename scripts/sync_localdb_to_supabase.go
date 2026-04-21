//go:build ignore

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
)

func main() {
	var (
		localDBPath string
		dsn         string
		pageSize    int
		batchSize   int
		dryRun      bool
	)

	flag.StringVar(&localDBPath, "local-db", "webdata\\leadsmanager.db", "path to local leads SQLite database")
	flag.StringVar(&dsn, "dsn", os.Getenv("DATABASE_URL"), "Supabase/Postgres DSN (defaults to DATABASE_URL)")
	flag.IntVar(&pageSize, "page-size", 500, "number of local rows fetched per page")
	flag.IntVar(&batchSize, "batch-size", 200, "number of leads per upsert batch")
	flag.BoolVar(&dryRun, "dry-run", false, "read local leads and report counts without writing to Supabase")
	flag.Parse()

	if pageSize <= 0 {
		log.Fatal("page-size must be > 0")
	}
	if batchSize <= 0 {
		log.Fatal("batch-size must be > 0")
	}

	ctx := context.Background()

	localDB, err := leadsmanager.NewDB(ctx, localDBPath)
	if err != nil {
		log.Fatalf("open local db: %v", err)
	}
	defer localDB.Close()

	localStats, err := localDB.GetStats(ctx)
	if err != nil {
		log.Fatalf("read local stats: %v", err)
	}
	log.Printf("local leads before sync: %d", localStats.TotalLeads)

	if dryRun {
		processed, err := countLocalLeads(ctx, localDB, pageSize)
		if err != nil {
			log.Fatalf("dry-run count local leads: %v", err)
		}
		log.Printf("dry-run complete: local leads ready for sync=%d", processed)
		return
	}

	if strings.TrimSpace(dsn) == "" {
		log.Fatal("dsn is required (pass -dsn or set DATABASE_URL)")
	}

	dsn = withSimpleProtocolDSN(dsn)

	remoteDB, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open remote db: %v", err)
	}
	defer remoteDB.Close()

	remoteDB.SetMaxOpenConns(5)
	remoteDB.SetMaxIdleConns(2)
	remoteDB.SetConnMaxLifetime(30 * time.Minute)

	if err := remoteDB.PingContext(ctx); err != nil {
		log.Fatalf("ping remote db: %v", err)
	}

	remoteBefore, err := remoteCount(ctx, remoteDB)
	if err != nil {
		log.Fatalf("count remote leads before sync: %v", err)
	}
	log.Printf("remote leads before sync: %d", remoteBefore)

	processed, saved, failed, err := syncLeads(ctx, localDB, remoteDB, pageSize, batchSize)
	if err != nil {
		log.Fatalf("sync leads: %v", err)
	}

	remoteAfter, err := remoteCount(ctx, remoteDB)
	if err != nil {
		log.Fatalf("count remote leads after sync: %v", err)
	}

	log.Printf("sync complete: processed=%d saved=%d failed=%d", processed, saved, failed)
	log.Printf("remote leads after sync: %d (delta %+d)", remoteAfter, remoteAfter-remoteBefore)
}

func countLocalLeads(ctx context.Context, localDB *leadsmanager.DB, pageSize int) (int, error) {
	processed := 0
	for page := 1; ; page++ {
		leads, _, err := localDB.FetchLeads(ctx, leadsmanager.LeadFilter{}, page, pageSize)
		if err != nil {
			return 0, err
		}
		if len(leads) == 0 {
			break
		}
		processed += len(leads)
	}
	return processed, nil
}

func syncLeads(ctx context.Context, localDB *leadsmanager.DB, remoteDB *sql.DB, pageSize, batchSize int) (int, int, int, error) {
	processed := 0
	saved := 0
	failed := 0

	for page := 1; ; page++ {
		leads, total, err := localDB.FetchLeads(ctx, leadsmanager.LeadFilter{}, page, pageSize)
		if err != nil {
			return processed, saved, failed, err
		}
		if len(leads) == 0 {
			break
		}

		for start := 0; start < len(leads); start += batchSize {
			end := start + batchSize
			if end > len(leads) {
				end = len(leads)
			}

			batch := leads[start:end]
			n, f, err := upsertBatchWithFallback(ctx, remoteDB, batch)
			if err != nil {
				return processed, saved, failed, err
			}
			saved += n
			failed += f
		}

		processed += len(leads)
		log.Printf("progress: processed=%d/%d saved=%d failed=%d", processed, total, saved, failed)
	}

	return processed, saved, failed, nil
}

func upsertBatchWithFallback(ctx context.Context, remoteDB *sql.DB, leads []leadsmanager.Lead) (int, int, error) {
	normalized := normalizeLeads(leads)
	query, args := buildInsertQueryAndArgs(normalized)
	if _, err := remoteDB.ExecContext(ctx, query, args...); err == nil {
		return len(normalized), 0, nil
	} else {
		log.Printf("batch upsert failed; retrying per-row: %v", err)
	}

	saved := 0
	failed := 0
	for _, lead := range normalized {
		singleQuery, singleArgs := buildInsertQueryAndArgs([]leadsmanager.Lead{lead})
		if _, err := remoteDB.ExecContext(ctx, singleQuery, singleArgs...); err != nil {
			failed++
			log.Printf("row upsert failed for place_id=%s: %v", lead.PlaceID, err)
			continue
		}
		saved++
	}

	return saved, failed, nil
}

func normalizeLeads(leads []leadsmanager.Lead) []leadsmanager.Lead {
	now := time.Now().UTC()
	out := make([]leadsmanager.Lead, 0, len(leads))
	for _, lead := range leads {
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
		out = append(out, lead)
	}
	return out
}

func buildInsertQueryAndArgs(leads []leadsmanager.Lead) (string, []any) {
	const numCols = 40

	var placeholders []string
	args := make([]any, 0, len(leads)*numCols)

	for i, lead := range leads {
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

	return query, args
}

func remoteCount(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.gmaps_leads").Scan(&n)
	return n, err
}

func withSimpleProtocolDSN(dsn string) string {
	if strings.Contains(dsn, "default_query_exec_mode") {
		return dsn
	}
	sep := " "
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		sep = "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
	}
	return dsn + sep + "default_query_exec_mode=simple_protocol"
}

func pqStringArray(ss []string) string {
	if len(ss) == 0 {
		return "{}"
	}
	escaped := make([]string, len(ss))
	for i, s := range ss {
		s = strings.ReplaceAll(s, `\\`, `\\\\`)
		s = strings.ReplaceAll(s, `"`, `\\"`)
		escaped[i] = `"` + s + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}"
}

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

