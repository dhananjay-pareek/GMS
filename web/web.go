package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed static
var static embed.FS

type Server struct {
	tmpl        map[string]*template.Template
	srv         *http.Server
	svc         *Service
	leadsURL    string
	llmProvider string
	llmAPIKey   string
	llmModel    string
	ollamaURL   string
}

func New(
	svc *Service,
	addr,
	leadsURL,
	llmProvider,
	llmAPIKey,
	llmModel,
	ollamaURL string,
) (*Server, error) {
	if leadsURL == "" {
		leadsURL = "http://localhost:9090"
	}
	if llmProvider == "" {
		llmProvider = "ollama"
	}
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	ans := Server{
		svc:         svc,
		tmpl:        make(map[string]*template.Template),
		leadsURL:    leadsURL,
		llmProvider: llmProvider,
		llmAPIKey:   llmAPIKey,
		llmModel:    llmModel,
		ollamaURL:   ollamaURL,
		srv: &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
	}

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(staticFS))
	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))
	mux.HandleFunc("/health", ans.health)
	mux.HandleFunc("/healthz", ans.health)
	mux.HandleFunc("/scrape", ans.scrape)
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		ans.download(w, r)
	})
	mux.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		ans.delete(w, r)
	})
	mux.HandleFunc("/jobs", ans.getJobs)
	mux.HandleFunc("/", ans.index)

	// api routes
	mux.HandleFunc("/api/generate-keywords", ans.apiGenerateKeywords)
	mux.HandleFunc("/api/docs", ans.redocHandler)
	mux.HandleFunc("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ans.apiScrape(w, r)
		case http.MethodGet:
			ans.apiGetJobs(w, r)
		default:
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)
		}
	})

	mux.HandleFunc("/api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		switch r.Method {
		case http.MethodGet:
			ans.apiGetJob(w, r)
		case http.MethodDelete:
			ans.apiDeleteJob(w, r)
		default:
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)
		}
	})

	mux.HandleFunc("/api/v1/jobs/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		if r.Method != http.MethodGet {
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)

			return
		}

		ans.download(w, r)
	})

	handler := securityHeaders(mux)
	ans.srv.Handler = handler

	tmplsKeys := []string{
		"static/templates/index.html",
		"static/templates/job_rows.html",
		"static/templates/job_row.html",
		"static/templates/redoc.html",
	}

	for _, key := range tmplsKeys {
		tmp, err := template.ParseFS(static, key)
		if err != nil {
			return nil, err
		}

		ans.tmpl[key] = tmp
	}

	return &ans, nil
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()

		err := s.srv.Shutdown(context.Background())
		if err != nil {
			log.Println(err)

			return
		}

		log.Println("server stopped")
	}()

	fmt.Fprintf(os.Stderr, "visit http://localhost%s\n", s.srv.Addr)

	err := s.srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

type formData struct {
	Name          string
	MaxTime       string
	Keywords      []string
	Language      string
	Zoom          int
	FastMode      bool
	Radius        int
	Lat           string
	Lon           string
	Depth         int
	Email         bool
	Proxies       []string
	LeadsURL      string
	LLMProvider   string
	LLMAPIKey     string
	LLMModel      string
}

type ctxKey string

const idCtxKey ctxKey = "id"

func requestWithID(r *http.Request) *http.Request {
	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}

	parsed, err := uuid.Parse(id)
	if err == nil {
		r = r.WithContext(context.WithValue(r.Context(), idCtxKey, parsed))
	}

	return r
}

func getIDFromRequest(r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(idCtxKey).(uuid.UUID)

	return id, ok
}

//nolint:gocritic // this is used in template
func (f formData) ProxiesString() string {
	return strings.Join(f.Proxies, "\n")
}

//nolint:gocritic // this is used in template
func (f formData) KeywordsString() string {
	return strings.Join(f.Keywords, "\n")
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	tmpl, ok := s.tmpl["static/templates/index.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	data := formData{
		Name:          "",
		MaxTime:       "10m",
		Keywords:      []string{},
		Language:      "en",
		Zoom:          15,
		FastMode:      false,
		Radius:        10000,
		Lat:           "0",
		Lon:           "0",
		Depth:         10,
		Email:         false,
		LeadsURL:      s.leadsURL,
		LLMProvider:   s.llmProvider,
		LLMAPIKey: func() string {
			if s.llmProvider == "ollama" && s.llmAPIKey == "" {
				return s.ollamaURL
			}

			return s.llmAPIKey
		}(),
		LLMModel: s.llmModel,
	}

	_ = tmpl.Execute(w, data)
}

func (s *Server) scrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	newJob := Job{
		ID:     uuid.New().String(),
		Name:   r.Form.Get("name"),
		Date:   kolkataTime(),
		Status: StatusPending,
		Data:   JobData{},
	}

	maxTimeStr := r.Form.Get("maxtime")

	maxTime, err := time.ParseDuration(maxTimeStr)
	if err != nil {
		http.Error(w, "invalid max time", http.StatusUnprocessableEntity)

		return
	}

	if maxTime < time.Minute*3 {
		http.Error(w, "max time must be more than 3m", http.StatusUnprocessableEntity)

		return
	}

	newJob.Data.MaxTime = maxTime

	keywordsStr, ok := r.Form["keywords"]
	if !ok {
		http.Error(w, "missing keywords", http.StatusUnprocessableEntity)

		return
	}

	keywords := strings.Split(keywordsStr[0], "\n")
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}

		newJob.Data.Keywords = append(newJob.Data.Keywords, k)
	}

	newJob.Data.Lang = r.Form.Get("lang")

	newJob.Data.Zoom, err = strconv.Atoi(r.Form.Get("zoom"))
	if err != nil {
		http.Error(w, "invalid zoom", http.StatusUnprocessableEntity)

		return
	}

	if r.Form.Get("fastmode") == "on" {
		newJob.Data.FastMode = true
	}

	newJob.Data.Radius, err = strconv.Atoi(r.Form.Get("radius"))
	if err != nil {
		http.Error(w, "invalid radius", http.StatusUnprocessableEntity)

		return
	}

	newJob.Data.Lat = r.Form.Get("latitude")
	newJob.Data.Lon = r.Form.Get("longitude")

	newJob.Data.Depth, err = strconv.Atoi(r.Form.Get("depth"))
	if err != nil {
		http.Error(w, "invalid depth", http.StatusUnprocessableEntity)

		return
	}

	newJob.Data.Email = r.Form.Get("email") == "on"

	proxies := strings.Split(r.Form.Get("proxies"), "\n")
	if len(proxies) > 0 {
		for _, p := range proxies {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			newJob.Data.Proxies = append(newJob.Data.Proxies, p)
		}
	}

	err = newJob.Validate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)

		return
	}

	err = s.svc.Create(r.Context(), &newJob)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	tmpl, ok := s.tmpl["static/templates/job_row.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	_ = tmpl.Execute(w, newJob)
}

func (s *Server) getJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	tmpl, ok := s.tmpl["static/templates/job_rows.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)
		return
	}

	jobs, err := s.svc.All(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	_ = tmpl.Execute(w, jobs)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	ctx := r.Context()

	id, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid ID", http.StatusUnprocessableEntity)

		return
	}

	filePath, err := s.svc.GetCSV(ctx, id.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Results CSV file not found (it may have been deleted or the job failed).", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fileName := filepath.Base(filePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	w.Header().Set("Content-Type", "text/csv")

	_, err = io.Copy(w, file)
	if err != nil {
		http.Error(w, "Failed to send file", http.StatusInternalServerError)
		return
	}
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	deleteID, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid ID", http.StatusUnprocessableEntity)

		return
	}

	err := s.svc.Delete(r.Context(), deleteID.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusOK)
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type apiScrapeRequest struct {
	Name string
	JobData
}

type apiScrapeResponse struct {
	ID string `json:"id"`
}

func (s *Server) redocHandler(w http.ResponseWriter, _ *http.Request) {
	tmpl, ok := s.tmpl["static/templates/redoc.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	_ = tmpl.Execute(w, nil)
}

func (s *Server) apiScrape(w http.ResponseWriter, r *http.Request) {
	var req apiScrapeRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ans := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusUnprocessableEntity, ans)

		return
	}

	newJob := Job{
		ID:     uuid.New().String(),
		Name:   req.Name,
		Date:   kolkataTime(),
		Status: StatusPending,
		Data:   req.JobData,
	}

	// convert to seconds
	newJob.Data.MaxTime *= time.Second

	err = newJob.Validate()
	if err != nil {
		ans := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusUnprocessableEntity, ans)

		return
	}

	err = s.svc.Create(r.Context(), &newJob)
	if err != nil {
		ans := apiError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusInternalServerError, ans)

		return
	}

	ans := apiScrapeResponse{
		ID: newJob.ID,
	}

	renderJSON(w, http.StatusCreated, ans)
}

func (s *Server) apiGetJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.svc.All(r.Context())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusInternalServerError, apiError)

		return
	}

	renderJSON(w, http.StatusOK, jobs)
}

func (s *Server) apiGetJob(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		apiError := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid ID",
		}

		renderJSON(w, http.StatusUnprocessableEntity, apiError)

		return
	}

	job, err := s.svc.Get(r.Context(), id.String())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusNotFound,
			Message: http.StatusText(http.StatusNotFound),
		}

		renderJSON(w, http.StatusNotFound, apiError)

		return
	}

	renderJSON(w, http.StatusOK, job)
}

func (s *Server) apiDeleteJob(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		apiError := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid ID",
		}

		renderJSON(w, http.StatusUnprocessableEntity, apiError)

		return
	}

	err := s.svc.Delete(r.Context(), id.String())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusInternalServerError, apiError)

		return
	}

	w.WriteHeader(http.StatusOK)
}

func renderJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	_ = json.NewEncoder(w).Encode(data)
}

func formatDate(t time.Time) string {
	return t.Format("Jan 02, 2006 15:04:05")
}

// kolkataTime converts UTC time to Asia/Kolkata timezone
func kolkataTime() time.Time {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		// Fallback: IST is UTC+5:30
		loc = time.FixedZone("IST", 5*60*60+30*60)
	}
	return time.Now().In(loc)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Basic health check - service is running
	response := map[string]interface{}{
		"status":    "ok",
		"service":   "google-maps-scraper",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' cdn.redoc.ly cdnjs.cloudflare.com unpkg.com 'unsafe-inline' 'unsafe-eval'; "+
				"worker-src 'self' blob:; "+
				"style-src 'self' 'unsafe-inline' fonts.googleapis.com unpkg.com; "+
				"img-src 'self' data: cdn.redoc.ly *.tile.openstreetmap.org unpkg.com; "+
				"font-src 'self' fonts.gstatic.com; "+
				"connect-src 'self' nominatim.openstreetmap.org unpkg.com generativelanguage.googleapis.com api.openai.com api.anthropic.com api.cohere.ai api.mistral.ai api.groq.com api.together.xyz api.replicate.com api-inference.huggingface.co api.perplexity.ai localhost:11434 127.0.0.1:11434")

		next.ServeHTTP(w, r)
	})
}

// ─── AI Keyword Generation ───────────────────────────────────────────────────

type generateKeywordsRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	JobName  string `json:"job_name"`
	Location string `json:"location"`
}

type generateKeywordsResponse struct {
	Keywords []string `json:"keywords,omitempty"`
	Error    string   `json:"error,omitempty"`
}

func (s *Server) apiGenerateKeywords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		renderJSON(w, http.StatusMethodNotAllowed, apiError{Code: 405, Message: "Method not allowed"})
		return
	}

	var req generateKeywordsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, generateKeywordsResponse{Error: "Invalid request body"})
		return
	}

	if req.JobName == "" && req.Location == "" {
		renderJSON(w, http.StatusBadRequest, generateKeywordsResponse{Error: "Job name or location is required"})
		return
	}

	prompt := buildKeywordPrompt(req.JobName, req.Location)

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = s.llmProvider
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = s.llmAPIKey
	}
	if provider == "ollama" && apiKey == "" {
		apiKey = s.ollamaURL
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = s.llmModel
	}

	if provider != "ollama" && apiKey == "" {
		renderJSON(w, http.StatusBadRequest, generateKeywordsResponse{Error: "API key is required"})
		return
	}

	result, err := callLLM(provider, apiKey, model, prompt)
	if err != nil {
		log.Printf("LLM call failed: %v", err)
		renderJSON(w, http.StatusInternalServerError, generateKeywordsResponse{Error: err.Error()})
		return
	}

	keywords := parseKeywords(result)
	renderJSON(w, http.StatusOK, generateKeywordsResponse{Keywords: keywords})
}

func buildKeywordPrompt(jobName, location string) string {
	var sb strings.Builder
	sb.WriteString("Generate 15-20 highly specific and broad Google Maps search keywords for scraping business leads.\n")
	sb.WriteString("Each keyword should be a realistic search query someone would type into Google Maps.\n")

	if jobName != "" {
		sb.WriteString(fmt.Sprintf("Business type / niche given by user: %s\n", jobName))
		sb.WriteString("Crucial Instruction: THINK BROADLY. Do not just repeat the exact niche. Include synonyms, related sub-industries, suppliers, manufacturers, distributors, and specific material/service variations. For example, if the niche is 'stone sellers', you MUST include terms like 'marble suppliers', 'granite fabricators', 'quartz countertops', 'masonry supply', 'landscaping rock yard', etc.\n")
	}
	if location != "" {
		sb.WriteString(fmt.Sprintf("Target location: %s\n", location))
	}

	sb.WriteString("\nRules:\n")
	sb.WriteString("- One keyword per line\n")
	sb.WriteString("- Include location in each keyword (or a variation of it)\n")
	sb.WriteString("- Mix broad category terms and highly specific niche terms\n")
	sb.WriteString("- Include related business types, materials, and services\n")
	sb.WriteString("- No numbering, no bullets, no explanations — ONLY the raw search queries, one per line\n")
	return sb.String()
}

func parseKeywords(raw string) []string {
	lines := strings.Split(raw, "\n")
	var keywords []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Strip leading numbering like "1. " or "- "
		if len(line) > 2 && (line[0] >= '0' && line[0] <= '9') {
			if idx := strings.IndexAny(line, ".)- "); idx > 0 && idx < 4 {
				line = strings.TrimSpace(line[idx+1:])
			}
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimSpace(line)
		if line != "" {
			keywords = append(keywords, line)
		}
	}
	return keywords
}

func callLLM(provider, apiKey, model, prompt string) (string, error) {
	switch provider {
	case "gemini":
		return callGemini(apiKey, model, prompt)
	case "openai":
		return callOpenAICompatible("https://api.openai.com/v1/chat/completions", apiKey, model, prompt, "gpt-4o-mini")
	case "anthropic":
		return callAnthropic(apiKey, model, prompt)
	case "groq":
		return callOpenAICompatible("https://api.groq.com/openai/v1/chat/completions", apiKey, model, prompt, "llama-3.1-8b-instant")
	case "together":
		return callOpenAICompatible("https://api.together.xyz/v1/chat/completions", apiKey, model, prompt, "meta-llama/Llama-3-8b-chat-hf")
	case "mistral":
		return callOpenAICompatible("https://api.mistral.ai/v1/chat/completions", apiKey, model, prompt, "mistral-small-latest")
	case "perplexity":
		return callOpenAICompatible("https://api.perplexity.ai/chat/completions", apiKey, model, prompt, "llama-3.1-sonar-small-128k-online")
	case "cohere":
		return callCohere(apiKey, model, prompt)
	case "ollama":
		host := apiKey // for Ollama, "api_key" field holds the host URL
		if model == "" {
			model = "qwen3-coder:480b-cloud"
		}
		return callOllama(host, model, prompt)
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

// ─── Gemini ──────────────────────────────────────────────────────────────────

func callGemini(apiKey, model, prompt string) (string, error) {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	body := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": prompt}}},
		},
	}

	respBody, err := doJSON(http.MethodPost, url, body, nil)
	if err != nil {
		return "", err
	}

	// Parse Gemini response
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse Gemini response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("Gemini API error: %s", result.Error.Message)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

// ─── OpenAI-compatible (OpenAI, Groq, Together, Mistral, Perplexity) ────────

func callOpenAICompatible(baseURL, apiKey, model, prompt, defaultModel string) (string, error) {
	if model == "" {
		model = defaultModel
	}

	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 1024,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
	}

	respBody, err := doJSON(http.MethodPost, baseURL, body, headers)
	if err != nil {
		return "", err
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Choices[0].Message.Content, nil
}

// ─── Anthropic ───────────────────────────────────────────────────────────────

func callAnthropic(apiKey, model, prompt string) (string, error) {
	if model == "" {
		model = "claude-3-5-haiku-latest"
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	headers := map[string]string{
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
	}

	respBody, err := doJSON(http.MethodPost, "https://api.anthropic.com/v1/messages", body, headers)
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse Anthropic response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("Anthropic error: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty Anthropic response")
	}
	return result.Content[0].Text, nil
}

// ─── Cohere ──────────────────────────────────────────────────────────────────

func callCohere(apiKey, model, prompt string) (string, error) {
	if model == "" {
		model = "command-r"
	}

	body := map[string]any{
		"model":   model,
		"message": prompt,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
	}

	respBody, err := doJSON(http.MethodPost, "https://api.cohere.ai/v1/chat", body, headers)
	if err != nil {
		return "", err
	}

	var result struct {
		Text    string `json:"text"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse Cohere response: %w", err)
	}
	if result.Text == "" && result.Message != "" {
		return "", fmt.Errorf("Cohere error: %s", result.Message)
	}
	return result.Text, nil
}

// ─── Ollama (local) ──────────────────────────────────────────────────────────

func callOllama(host, model, prompt string) (string, error) {
	if !strings.HasPrefix(host, "http") {
		host = "http://" + host
	}
	host = strings.TrimRight(host, "/")

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	}

	respBody, err := doJSON(http.MethodPost, host+"/api/generate", body, nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse Ollama response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("Ollama error: %s", result.Error)
	}
	return result.Response, nil
}

// ─── HTTP helper ─────────────────────────────────────────────────────────────

func doJSON(method, url string, body any, headers map[string]string) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, nil
}
