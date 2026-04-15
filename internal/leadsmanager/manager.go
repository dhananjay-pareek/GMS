package leadsmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gosom/google-maps-scraper/gmaps"
)

// Manager coordinates the leads pipeline and database operations.
type Manager struct {
	db          *DB
	enricher    *Enricher
	llmProvider string
	llmAPIKey   string
	llmModel    string
	ollamaURL   string
}

// NewManager creates a new Manager instance.
func NewManager(db *DB, llmProvider, llmAPIKey, llmModel, ollamaURL string) (*Manager, error) {
	enricher, err := NewEnricher(db)
	if err != nil {
		return nil, fmt.Errorf("init enricher: %w", err)
	}

	if llmProvider == "" {
		llmProvider = "ollama"
	}
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	return &Manager{
		db:          db,
		enricher:    enricher,
		llmProvider: llmProvider,
		llmAPIKey:   llmAPIKey,
		llmModel:    llmModel,
		ollamaURL:   ollamaURL,
	}, nil
}

// DB returns the database instance for direct queries.
func (m *Manager) DB() *DB {
	return m.db
}

// ImportRequest represents the JSON body for the import endpoint.
type ImportRequest struct {
	Entries []gmaps.Entry `json:"entries"`
}

// ImportResponse contains the result of an import operation.
type ImportResponse struct {
	Imported int      `json:"imported"`
	Errors   []string `json:"errors,omitempty"`
}

// LeadsResponse is the paginated response for the leads API.
type LeadsResponse struct {
	Leads    []Lead `json:"leads"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Pages    int    `json:"pages"`
}

// HandleImport processes incoming scraped data via POST /api/import.
func (m *Manager) HandleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if len(req.Entries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no entries provided"})
		return
	}

	resp := ImportResponse{}
	var leads []Lead

	for _, entry := range req.Entries {
		if entry.PlaceID == "" {
			resp.Errors = append(resp.Errors, fmt.Sprintf("skipping entry '%s': missing place_id", entry.Title))
			continue
		}

		lead := ProcessEntry(entry)
		leads = append(leads, lead)
	}

	if len(leads) > 0 {
		// Use a background context with timeout for DB operations
		// This prevents the HTTP request timeout from canceling the DB operation
		dbCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		if err := m.db.UpsertLeads(dbCtx, leads); err != nil {
			log.Printf("error upserting leads: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error: " + err.Error()})
			return
		}

		log.Printf("Successfully imported %d leads", len(leads))
	}

	resp.Imported = len(leads)
	writeJSON(w, http.StatusOK, resp)
}

// HandleLeads returns paginated leads via GET /api/leads.
func (m *Manager) HandleLeads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	filter := LeadFilter{
		Search:   q.Get("search"),
		City:     q.Get("city"),
		Category: q.Get("category"),
		Tag:      q.Get("tag"),
	}

	if minR := q.Get("min_rating"); minR != "" {
		if v, err := strconv.ParseFloat(minR, 64); err == nil {
			filter.MinRating = v
		}
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}

	leads, total, err := m.db.FetchLeads(r.Context(), filter, page, pageSize)
	if err != nil {
		log.Printf("error fetching leads: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}

	resp := LeadsResponse{
		Leads:    leads,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleLead returns a single lead via GET /api/leads/{place_id}.
func (m *Manager) HandleLead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	placeID := r.PathValue("place_id")
	if placeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing place_id"})
		return
	}

	lead, err := m.db.GetLead(r.Context(), placeID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, lead)
}

// HandleStats returns aggregate stats for the dashboard header.
func (m *Manager) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	stats, err := m.getStats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

type DashboardStats struct {
	TotalLeads   int     `json:"total_leads"`
	WithWebsite  int     `json:"with_website"`
	WithEmail    int     `json:"with_email"`
	AvgRating    float64 `json:"avg_rating"`
	FlaggedCount int     `json:"flagged_count"`
}

func (m *Manager) getStats(ctx context.Context) (*DashboardStats, error) {
	return m.db.GetStats(ctx)
}

// HandleTechStack runs tech stack detection on a lead.
func (m *Manager) HandleTechStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	placeID := r.URL.Query().Get("place_id")
	if placeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing place_id"})
		return
	}

	techs, err := m.enricher.DetectTechStack(r.Context(), placeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"place_id": placeID, "tech_stack": techs})
}

// HandlePageSpeed runs PageSpeed Insights on a lead's website.
func (m *Manager) HandlePageSpeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	placeID := r.URL.Query().Get("place_id")
	if placeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing place_id"})
		return
	}

	result, err := m.enricher.RunPageSpeed(r.Context(), placeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleScreenshot returns a screenshot URL for a lead's website.
func (m *Manager) HandleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	placeID := r.URL.Query().Get("place_id")
	if placeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing place_id"})
		return
	}

	result, err := m.enricher.TakeScreenshot(r.Context(), placeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleContacts extracts contacts from a lead's website.
func (m *Manager) HandleContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	placeID := r.URL.Query().Get("place_id")
	if placeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing place_id"})
		return
	}

	result, err := m.enricher.ExtractContacts(r.Context(), placeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandlePitchPrompt generates an AI pitch prompt for a lead.
func (m *Manager) HandlePitchPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	placeID := r.URL.Query().Get("place_id")
	persona := r.URL.Query().Get("persona")
	apiKey := r.URL.Query().Get("api_key")
	provider := r.URL.Query().Get("provider")
	modelName := r.URL.Query().Get("model")
	if provider == "" {
		provider = m.llmProvider
	}
	if apiKey == "" {
		apiKey = m.llmAPIKey
	}
	if provider == "ollama" && apiKey == "" {
		apiKey = m.ollamaURL
	}
	if modelName == "" {
		modelName = m.llmModel
	}
	if placeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing place_id"})
		return
	}
	if persona == "" {
		persona = "marketing consultant"
	}

	prompt, err := m.enricher.GeneratePitchPrompt(r.Context(), placeID, persona, apiKey, provider, modelName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"prompt": prompt, "place_id": placeID})
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
