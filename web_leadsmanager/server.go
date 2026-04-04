package web_leadsmanager

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gosom/google-maps-scraper/internal/leadsmanager"
)

//go:embed static
var static embed.FS

// Server serves the Leads Manager dashboard and API.
type Server struct {
	tmpl map[string]*template.Template
	srv  *http.Server
	mgr  *leadsmanager.Manager
}

// New creates a new Leads Manager web server.
func New(mgr *leadsmanager.Manager, addr string) (*Server, error) {
	s := &Server{
		mgr:  mgr,
		tmpl: make(map[string]*template.Template),
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

// DashboardData holds all data for the dashboard template.
type DashboardData struct {
	Leads    []leadsmanager.Lead
	Stats    *leadsmanager.DashboardStats
	Total    int
	Page     int
	PageSize int
	Pages    int
	Search   string
	City     string
	Category string
	Tag      string
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

	_ = tmpl.ExecuteTemplate(w, "leads.html", nil)
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
		Leads:    leads,
		Total:    total,
		Page:     page,
		PageSize: 25,
		Pages:    pages,
		Search:   filter.Search,
		City:     filter.City,
		Category: filter.Category,
		Tag:      filter.Tag,
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
				"img-src 'self' data:; "+
				"connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
