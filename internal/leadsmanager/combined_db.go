package leadsmanager

import (
	"context"
	"log"
	"sort"
	"sync"
)

// CombinedDB merges leads from a local SQLite DB and an optional Supabase DB.
// Reads are fanout across both sources; writes go to the local DB only.
type CombinedDB struct {
	local  *DB
	remote *SupabaseDB // nil if DATABASE_URL not set
}

// NewCombinedDB creates a CombinedDB. remote may be nil (local-only mode).
func NewCombinedDB(local *DB, remote *SupabaseDB) *CombinedDB {
	return &CombinedDB{local: local, remote: remote}
}

// Close closes both underlying connections.
func (c *CombinedDB) Close() {
	c.local.Close()
	if c.remote != nil {
		c.remote.Close()
	}
}

// FetchLeads fetches from local and Supabase in parallel, merges by place_id
// (local record wins), and returns the unified set with a combined total.
func (c *CombinedDB) FetchLeads(ctx context.Context, filter LeadFilter, page, pageSize int) ([]Lead, int, error) {
	if c.remote == nil {
		return c.local.FetchLeads(ctx, filter, page, pageSize)
	}

	type result struct {
		leads []Lead
		total int
		err   error
	}

	localCh := make(chan result, 1)
	remoteCh := make(chan result, 1)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		leads, total, err := c.local.FetchLeads(ctx, filter, 1, 10000) // fetch all for merge
		localCh <- result{leads, total, err}
	}()

	go func() {
		defer wg.Done()
		leads, total, err := c.remote.FetchLeads(ctx, filter, 1, 10000) // fetch all for merge
		if err != nil {
			log.Printf("combined_db: supabase FetchLeads error (non-fatal): %v", err)
			leads, total, err = nil, 0, nil
		}
		remoteCh <- result{leads, total, err}
	}()

	wg.Wait()
	close(localCh)
	close(remoteCh)

	lr := <-localCh
	rr := <-remoteCh

	log.Printf("combined_db: FetchLeads counts -> local: %d, remote: %d (total: %d)", len(lr.leads), len(rr.leads), rr.total)

	if lr.err != nil {
		return nil, 0, lr.err
	}

	// Merge: local wins on duplicate place_id
	seen := make(map[string]struct{}, len(lr.leads)+len(rr.leads))
	merged := make([]Lead, 0, len(lr.leads)+len(rr.leads))

	for _, lead := range lr.leads {
		seen[lead.PlaceID] = struct{}{}
		merged = append(merged, lead)
	}
	for _, lead := range rr.leads {
		if _, exists := seen[lead.PlaceID]; !exists {
			seen[lead.PlaceID] = struct{}{}
			merged = append(merged, lead)
		}
	}

	// Sort by updated_at DESC (same order as individual queries)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].UpdatedAt.After(merged[j].UpdatedAt)
	})

	total := len(merged)

	// Apply pagination on the merged set
	if pageSize < 1 {
		pageSize = 25
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if offset >= len(merged) {
		return []Lead{}, total, nil
	}
	if end > len(merged) {
		end = len(merged)
	}

	return merged[offset:end], total, nil
}

// GetLead tries local first, then Supabase.
func (c *CombinedDB) GetLead(ctx context.Context, placeID string) (*Lead, error) {
	lead, err := c.local.GetLead(ctx, placeID)
	if err == nil {
		return lead, nil
	}
	if c.remote != nil {
		return c.remote.GetLead(ctx, placeID)
	}
	return nil, err
}

// GetStats sums stats from both sources, avoiding double-counting by
// fetching all place_ids from each source.
func (c *CombinedDB) GetStats(ctx context.Context) (*DashboardStats, error) {
	localStats, err := c.local.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	if c.remote == nil {
		return localStats, nil
	}

	remoteStats, err := c.remote.GetStats(ctx)
	if err != nil {
		log.Printf("combined_db: supabase GetStats error (non-fatal): %v", err)
		return localStats, nil
	}

	// We can't deduplicate stats perfectly without fetching all place_ids,
	// so we do a best-effort sum and log a note. In practice local and Supabase
	// overlap heavily (dual-write), so we take the max of each metric instead.
	merged := &DashboardStats{
		TotalLeads:   max(localStats.TotalLeads, remoteStats.TotalLeads),
		WithWebsite:  max(localStats.WithWebsite, remoteStats.WithWebsite),
		WithEmail:    max(localStats.WithEmail, remoteStats.WithEmail),
		FlaggedCount: max(localStats.FlaggedCount, remoteStats.FlaggedCount),
	}
	// Weighted average rating (by record count)
	totalLeads := localStats.TotalLeads + remoteStats.TotalLeads
	if totalLeads > 0 {
		merged.AvgRating = (localStats.AvgRating*float64(localStats.TotalLeads) +
			remoteStats.AvgRating*float64(remoteStats.TotalLeads)) /
			float64(totalLeads)
	}
	return merged, nil
}

// GetCompetitors merges from both sources.
func (c *CombinedDB) GetCompetitors(ctx context.Context, city string, minRating float64) ([]Lead, error) {
	leads, err := c.local.GetCompetitors(ctx, city, minRating)
	if err != nil {
		return nil, err
	}
	if c.remote == nil {
		return leads, nil
	}
	remote, err := c.remote.GetCompetitors(ctx, city, minRating)
	if err != nil {
		log.Printf("combined_db: supabase GetCompetitors error (non-fatal): %v", err)
		return leads, nil
	}
	seen := make(map[string]struct{}, len(leads))
	for _, l := range leads {
		seen[l.PlaceID] = struct{}{}
	}
	for _, l := range remote {
		if _, ok := seen[l.PlaceID]; !ok {
			leads = append(leads, l)
		}
	}
	return leads, nil
}

// GetCompetitorsByCategory merges from both sources.
func (c *CombinedDB) GetCompetitorsByCategory(ctx context.Context, city, category, excludePlaceID string, minRating float64) ([]Lead, error) {
	leads, err := c.local.GetCompetitorsByCategory(ctx, city, category, excludePlaceID, minRating)
	if err != nil {
		return nil, err
	}
	if c.remote == nil {
		return leads, nil
	}
	remote, err := c.remote.GetCompetitorsByCategory(ctx, city, category, excludePlaceID, minRating)
	if err != nil {
		log.Printf("combined_db: supabase GetCompetitorsByCategory error (non-fatal): %v", err)
		return leads, nil
	}
	seen := make(map[string]struct{}, len(leads))
	for _, l := range leads {
		seen[l.PlaceID] = struct{}{}
	}
	for _, l := range remote {
		if _, ok := seen[l.PlaceID]; !ok {
			leads = append(leads, l)
		}
	}
	return leads, nil
}

// Write operations delegate to local DB only.
func (c *CombinedDB) UpsertLeads(ctx context.Context, leads []Lead) error {
	return c.local.UpsertLeads(ctx, leads)
}
func (c *CombinedDB) UpdateTechStack(ctx context.Context, placeID string, techs []string) error {
	return c.local.UpdateTechStack(ctx, placeID, techs)
}
func (c *CombinedDB) UpdatePageSpeedScore(ctx context.Context, placeID string, score int) error {
	return c.local.UpdatePageSpeedScore(ctx, placeID, score)
}
func (c *CombinedDB) UpdateEmails(ctx context.Context, placeID string, emails []string, isValid bool) error {
	return c.local.UpdateEmails(ctx, placeID, emails, isValid)
}

func (c *CombinedDB) UpdateCallStatus(ctx context.Context, placeID, calledBy, response string) error {
	err := c.local.UpdateCallStatus(ctx, placeID, calledBy, response)
	if c.remote != nil {
		if rerr := c.remote.UpdateCallStatus(ctx, placeID, calledBy, response); rerr != nil {
			log.Printf("combined_db: supabase UpdateCallStatus error (non-fatal): %v", rerr)
		}
	}
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
