package web_leadsmanager

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
)

//go:embed static
var static embed.FS

// Server serves the Leads Manager dashboard and API.
type Server struct {
	tmpl        map[string]*template.Template
	srv         *http.Server
	mgr         *leadsmanager.Manager
	scraperURL  string
	llmProvider string
	llmAPIKey   string
	llmModel    string
	ollamaURL   string
}

// New creates a new Leads Manager web server.
func New(
	mgr *leadsmanager.Manager,
	addr,
	scraperURL,
	llmProvider,
	llmAPIKey,
	llmModel,
	ollamaURL string,
) (*Server, error) {
	if scraperURL == "" {
		scraperURL = "http://localhost:8080"
	}
	if llmProvider == "" {
		llmProvider = "ollama"
	}
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	s := &Server{
		mgr:         mgr,
		tmpl:        make(map[string]*template.Template),
		scraperURL:  scraperURL,
		llmProvider: llmProvider,
		llmAPIKey:   llmAPIKey,
		llmModel:    llmModel,
		ollamaURL:   ollamaURL,
		srv: &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      120 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
	}

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		return nil, err
	}

	funcMap := template.FuncMap{
		"formatRating": leadsmanager.FormatRating,
		"add":          func(a, b int) int { return a + b },
		"sub":          func(a, b int) int { return a - b },
		"seq": func(start, end int) []int {
			s := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s
		},
		"joinTags": func(tags []string) string {
			result := ""
			for i, tag := range tags {
				if i > 0 {
					result += " "
				}
				result += tag
			}
			return result
		},
		"boolIcon": func(b *bool) string {
			if b == nil {
				return "—"
			}
			if *b {
				return "✅"
			}
			return "❌"
		},
		"truncate": func(s string, maxLen int) string {
			if len(s) <= maxLen {
				return s
			}
			return s[:maxLen] + "…"
		},
	}

	fileServer := http.FileServer(http.FS(staticFS))
	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	// Dashboard
	mux.HandleFunc("/", s.dashboard)
	mux.HandleFunc("/lead-rows", s.leadRows)

	// API
	mux.HandleFunc("/api/import", mgr.HandleImport)
	mux.HandleFunc("/api/leads", mgr.HandleLeads)
	mux.HandleFunc("/api/leads/{place_id}", mgr.HandleLead)
	mux.HandleFunc("POST /api/leads/{place_id}/call", mgr.HandleUpdateCallStatus)
	mux.HandleFunc("/api/stats", mgr.HandleStats)

	// On-demand enrichment
	mux.HandleFunc("/api/enrich/techstack", mgr.HandleTechStack)
	mux.HandleFunc("/api/enrich/pagespeed", mgr.HandlePageSpeed)
	mux.HandleFunc("/api/enrich/screenshot", mgr.HandleScreenshot)
	mux.HandleFunc("/api/enrich/contacts", mgr.HandleContacts)
	mux.HandleFunc("/api/enrich/pitch", mgr.HandlePitchPrompt)

	mux.HandleFunc("/health", s.health)

	handler := securityHeaders(mux)
	s.srv.Handler = handler

	// Parse templates
	tmplKeys := []string{
		"static/templates/leads.html",
		"static/templates/lead_rows.html",
	}

	for _, key := range tmplKeys {
		tmp, err := template.New("").Funcs(funcMap).ParseFS(static, key)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", key, err)
		}
		s.tmpl[key] = tmp
	}

	return s, nil
}

// Start begins serving HTTP requests.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		if err := s.srv.Shutdown(context.Background()); err != nil {
			log.Println("shutdown error:", err)
		}
	}()

	fmt.Printf("🚀 Leads Manager → http://localhost%s\n", s.srv.Addr)

	err := s.srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Addr returns the configured bind address (e.g. ":9090").
func (s *Server) Addr() string {
	return s.srv.Addr
}

// DashboardData holds all data for the dashboard template.
type DashboardData struct {
	Leads          []leadsmanager.Lead
	Stats          *leadsmanager.DashboardStats
	Total          int
	Page           int
	PageSize       int
	Pages          int
	Search         string
	City           string
	Category       string
	Tag            string
	IsCalledFilter string
}

type SelectOption struct {
	Value    string
	Selected bool
}

type CityGroup struct {
	Label   string
	Options []SelectOption
}

type CategoryGroup struct {
	Label   string
	Options []SelectOption
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl, ok := s.tmpl["static/templates/leads.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	categoryValues, err := s.mgr.DB().GetCategories(r.Context())
	if err != nil {
		log.Printf("dashboard: load categories failed: %v", err)
		categoryValues = []string{}
	}
	cityValues, err := s.mgr.DB().GetCities(r.Context())
	if err != nil {
		log.Printf("dashboard: load cities failed: %v", err)
		cityValues = []string{}
	}
	q := r.URL.Query()
	isCalled := q.Get("is_called")
	if isCalled == "" {
		isCalled = "false"
	}
	cityGroups := buildCityGroups(cityValues, q.Get("city"))
	categoryGroups := buildCategoryGroups(categoryValues, q.Get("category"))
	tagOptions := buildSelectOptions([]string{
		"No Website",
		"No SSL",
		"No Analytics",
		"Missing Pixel",
		"Needs SEO",
		"Low Rating",
		"Few Reviews",
		"Unclaimed GMB",
		"No Email",
		"No Phone",
		"Slow Website",
	}, q.Get("tag"))

	_ = tmpl.ExecuteTemplate(w, "leads.html", map[string]any{
		"ScraperURL":  s.scraperURL,
		"LLMProvider": s.llmProvider,
		"LLMAPIKey": func() string {
			if s.llmProvider == "ollama" && s.llmAPIKey == "" {
				return s.ollamaURL
			}
			return s.llmAPIKey
		}(),
		"LLMModel":           s.llmModel,
		"CategoryGroups":     categoryGroups,
		"CityGroups":         cityGroups,
		"TagOptions":         tagOptions,
		"Search":             q.Get("search"),
		"City":               q.Get("city"),
		"Category":           q.Get("category"),
		"Tag":                q.Get("tag"),
		"IsCalled":           isCalled,
		"InitialLeadRowsURL": buildLeadRowsURL(q),
	})
}

func buildLeadRowsURL(q url.Values) string {
	params := url.Values{}
	if v := q.Get("search"); v != "" {
		params.Set("search", v)
	}
	if v := q.Get("city"); v != "" {
		params.Set("city", v)
	}
	if v := q.Get("category"); v != "" {
		params.Set("category", v)
	}
	if v := q.Get("tag"); v != "" {
		params.Set("tag", v)
	}
	if v := q.Get("is_called"); v != "" {
		params.Set("is_called", v)
	} else {
		params.Set("is_called", "false")
	}
	return "/lead-rows?" + params.Encode()
}

func buildSelectOptions(values []string, selectedValue string) []SelectOption {
	options := make([]SelectOption, 0, len(values))
	for _, value := range values {
		options = append(options, SelectOption{Value: value, Selected: value == selectedValue})
	}
	return options
}

func buildCityGroups(values []string, selectedValue string) []CityGroup {
	const (
		delhiNCRLabel = "Delhi NCR"
		mumbaiLabel   = "Mumbai Metropolitan Region"
		dubaiLabel    = "Dubai / UAE"
		newYorkLabel  = "New York Metro"
		otherLabel    = "Other"
	)

	groupValues := map[string][]string{
		delhiNCRLabel: {},
		mumbaiLabel:   {},
		dubaiLabel:    {},
		newYorkLabel:  {},
		otherLabel:    {},
	}

	for _, value := range values {
		switch cityGroupLabel(value) {
		case delhiNCRLabel:
			groupValues[delhiNCRLabel] = append(groupValues[delhiNCRLabel], value)
		case mumbaiLabel:
			groupValues[mumbaiLabel] = append(groupValues[mumbaiLabel], value)
		case dubaiLabel:
			groupValues[dubaiLabel] = append(groupValues[dubaiLabel], value)
		case newYorkLabel:
			groupValues[newYorkLabel] = append(groupValues[newYorkLabel], value)
		default:
			groupValues[otherLabel] = append(groupValues[otherLabel], value)
		}
	}

	groupOrder := []string{delhiNCRLabel, mumbaiLabel, dubaiLabel, newYorkLabel, otherLabel}
	groupLabels := map[string]struct{}{}
	for label, vals := range groupValues {
		if len(vals) > 0 {
			groupLabels[label] = struct{}{}
		}
	}

	groups := make([]CityGroup, 0, len(groupLabels))
	for _, label := range groupOrder {
		vals := groupValues[label]
		if len(vals) == 0 {
			continue
		}
		groups = append(groups, CityGroup{Label: label, Options: buildSelectOptions(vals, selectedValue)})
	}

	return groups
}

func buildCategoryGroups(values []string, selectedValue string) []CategoryGroup {
	const (
		dentalLabel   = "Dental & Oral Health"
		jewelryLabel  = "Jewelry & Gemstones"
		businessLabel = "Marketing & Business Services"
		autoLabel     = "Auto Services"
		medicalLabel  = "Medical & Health"
		otherLabel    = "Other"
	)

	groupValues := map[string][]string{
		dentalLabel:   {},
		jewelryLabel:  {},
		businessLabel: {},
		autoLabel:     {},
		medicalLabel:  {},
		otherLabel:    {},
	}

	for _, value := range values {
		switch categoryGroupLabel(value) {
		case dentalLabel:
			groupValues[dentalLabel] = append(groupValues[dentalLabel], value)
		case jewelryLabel:
			groupValues[jewelryLabel] = append(groupValues[jewelryLabel], value)
		case businessLabel:
			groupValues[businessLabel] = append(groupValues[businessLabel], value)
		case autoLabel:
			groupValues[autoLabel] = append(groupValues[autoLabel], value)
		case medicalLabel:
			groupValues[medicalLabel] = append(groupValues[medicalLabel], value)
		default:
			groupValues[otherLabel] = append(groupValues[otherLabel], value)
		}
	}

	groupOrder := []string{dentalLabel, jewelryLabel, businessLabel, autoLabel, medicalLabel, otherLabel}
	groups := make([]CategoryGroup, 0, len(groupOrder))
	for _, label := range groupOrder {
		vals := groupValues[label]
		if len(vals) == 0 {
			continue
		}
		groups = append(groups, CategoryGroup{Label: label, Options: buildSelectOptions(vals, selectedValue)})
	}

	return groups
}

func categoryGroupLabel(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))

	switch {
	case strings.Contains(lower, "dental") || strings.Contains(lower, "orthodontist") || strings.Contains(lower, "periodontist") || strings.Contains(lower, "endodontist") || strings.Contains(lower, "oral and maxillofacial") || strings.Contains(lower, "prosthodontist") || strings.Contains(lower, "dental radiology") || strings.Contains(lower, "dental laboratory") || strings.Contains(lower, "dental school") || strings.Contains(lower, "dental implants") || strings.Contains(lower, "emergency dental") || strings.Contains(lower, "cosmetic dentist") || strings.Contains(lower, "pediatric dentist"):
		return "Dental & Oral Health"
	case strings.Contains(lower, "jewelry") || strings.Contains(lower, "jeweler") || strings.Contains(lower, "gemologist") || strings.Contains(lower, "goldsmith") || strings.Contains(lower, "gold dealer") || strings.Contains(lower, "silversmith") || strings.Contains(lower, "gemstone") || strings.Contains(lower, "natural stone"):
		return "Jewelry & Gemstones"
	case strings.Contains(lower, "marketing") || strings.Contains(lower, "advertising") || strings.Contains(lower, "website designer") || strings.Contains(lower, "software company") || strings.Contains(lower, "internet marketing") || strings.Contains(lower, "consultant") || strings.Contains(lower, "agency") || strings.Contains(lower, "call center") || strings.Contains(lower, "recruiter") || strings.Contains(lower, "wholesaler"):
		return "Marketing & Business Services"
	case strings.Contains(lower, "car ") || strings.Contains(lower, "auto") || strings.Contains(lower, "vehicle wrapping") || strings.Contains(lower, "car wash"):
		return "Auto Services"
	case strings.Contains(lower, "doctor") || strings.Contains(lower, "medical") || strings.Contains(lower, "surgical"):
		return "Medical & Health"
	default:
		return "Other"
	}
}

func cityGroupLabel(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))

	switch {
	case strings.Contains(lower, "dubai") || strings.Contains(lower, "al quoz") || strings.Contains(lower, "al barsha") || strings.Contains(lower, "al khawaneej") || strings.Contains(lower, "al satwa") || strings.Contains(lower, "jumeirah") || strings.Contains(lower, "ras al khor") || strings.Contains(lower, "umm ramool") || strings.Contains(lower, "marsa dubai") || strings.Contains(lower, "serkal") || strings.Contains(lower, "oasis center mall"):
		return "Dubai / UAE"
	case strings.Contains(lower, "new york") || strings.Contains(lower, "brooklyn") || strings.Contains(lower, "queens") || strings.Contains(lower, "bronx") || strings.Contains(lower, "staten island") || strings.Contains(lower, "long island city") || strings.Contains(lower, "jersey city") || strings.Contains(lower, "white plains") || strings.Contains(lower, "stamford") || strings.Contains(lower, "scarsdale") || strings.Contains(lower, "bethpage") || strings.Contains(lower, "elmhurst") || strings.Contains(lower, "cambria heights") || strings.Contains(lower, "neptune city") || strings.Contains(lower, "rockaway township") || strings.Contains(lower, "livingston") || strings.Contains(lower, "manalapan township") || strings.Contains(lower, "branchburg") || strings.Contains(lower, "chester") || strings.Contains(lower, "saylorsburg"):
		return "New York Metro"
	case strings.Contains(lower, "delhi") || strings.Contains(lower, "noida") || strings.Contains(lower, "gurugram") || strings.Contains(lower, "gurgaon") || strings.Contains(lower, "faridabad") || strings.Contains(lower, "ghaziabad"):
		return "Delhi NCR"
	case strings.Contains(lower, "mumbai") || strings.Contains(lower, "thane") || strings.Contains(lower, "panvel") || strings.Contains(lower, "mira bhayandar") || strings.Contains(lower, "navi mumbai") || strings.Contains(lower, "wahal"):
		return "Mumbai Metropolitan Region"
	default:
		return "Other"
	}
}

func (s *Server) leadRows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl, ok := s.tmpl["static/templates/lead_rows.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	filter := leadsmanager.LeadFilter{
		Search:   q.Get("search"),
		City:     q.Get("city"),
		Category: q.Get("category"),
		Tag:      q.Get("tag"),
	}

	isCalledStr := q.Get("is_called")
	if isCalledStr != "" {
		v := isCalledStr == "true"
		filter.IsCalled = &v
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	leads, total, err := s.mgr.DB().FetchLeads(r.Context(), filter, page, 25)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pages := total / 25
	if total%25 != 0 {
		pages++
	}

	data := DashboardData{
		Leads:          leads,
		Total:          total,
		Page:           page,
		PageSize:       25,
		Pages:          pages,
		Search:         filter.Search,
		City:           filter.City,
		Category:       filter.Category,
		Tag:            filter.Tag,
		IsCalledFilter: isCalledStr,
	}

	_ = tmpl.ExecuteTemplate(w, "lead_rows.html", data)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"leads-manager"}`))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' cdnjs.cloudflare.com 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline' fonts.googleapis.com; "+
				"font-src 'self' fonts.gstatic.com; "+
				"img-src 'self' data: https://*.thum.io https://*.googleusercontent.com; "+
				"connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
