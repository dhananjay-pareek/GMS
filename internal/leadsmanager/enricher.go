package leadsmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	wappalyzer "github.com/projectdiscovery/wappalyzergo"
)

// Enricher handles on-demand enrichment tasks for leads.
type Enricher struct {
	db         LeadStore
	httpClient *http.Client
	wappalyze  *wappalyzer.Wappalyze
}

// NewEnricher creates a new enrichment engine.
func NewEnricher(db LeadStore) (*Enricher, error) {
	wap, err := wappalyzer.New()
	if err != nil {
		return nil, fmt.Errorf("init wappalyzer: %w", err)
	}

	return &Enricher{
		db: db,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		wappalyze: wap,
	}, nil
}

// DetectTechStack identifies technologies used on a website.
func (e *Enricher) DetectTechStack(ctx context.Context, placeID string) ([]string, error) {
	lead, err := e.db.GetLead(ctx, placeID)
	if err != nil {
		return nil, err
	}

	if lead.Website == "" {
		return nil, fmt.Errorf("lead has no website")
	}

	url := lead.Website
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch website: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}

	// Fingerprint with headers + body
	fingerprints := e.wappalyze.Fingerprint(resp.Header, body)

	var techs []string
	for tech := range fingerprints {
		techs = append(techs, tech)
	}

	// Update in DB
	execErr := e.db.UpdateTechStack(ctx, placeID, techs)
	if execErr != nil {
		return techs, fmt.Errorf("saved techs but failed db update: %w", execErr)
	}

	return techs, nil
}

// PageSpeedResult holds Google PageSpeed API results.
type PageSpeedResult struct {
	Score       int    `json:"score"`
	FCP         string `json:"fcp"`
	LCP         string `json:"lcp"`
	CLS         string `json:"cls"`
	SpeedIndex  string `json:"speed_index"`
	Interactive string `json:"interactive"`
}

// RunPageSpeed calls the Google PageSpeed Insights API (free, no key required for basic).
func (e *Enricher) RunPageSpeed(ctx context.Context, placeID string) (*PageSpeedResult, error) {
	lead, err := e.db.GetLead(ctx, placeID)
	if err != nil {
		return nil, err
	}

	if lead.Website == "" {
		return nil, fmt.Errorf("lead has no website")
	}

	websiteURL := strings.TrimSpace(lead.Website)
	if !strings.HasPrefix(websiteURL, "http") {
		websiteURL = "https://" + websiteURL
	}

	values := url.Values{}
	values.Set("url", websiteURL)
	values.Set("strategy", "mobile")

	// Optional API key improves reliability and quota.
	if apiKey := strings.TrimSpace(os.Getenv("PAGESPEED_API_KEY")); apiKey != "" {
		values.Set("key", apiKey)
	}

	apiURL := "https://www.googleapis.com/pagespeedonline/v5/runPagespeed?" + values.Encode()

	pagespeedClient := &http.Client{Timeout: 45 * time.Second}

	var (
		resp        *http.Response
		bodyPreview string
		lastErr     error
	)

	for attempt := 1; attempt <= 2; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if reqErr != nil {
			return nil, reqErr
		}

		resp, lastErr = pagespeedClient.Do(req)
		if lastErr != nil {
			// One retry for transient network/timeout failures.
			if attempt < 2 {
				time.Sleep(2 * time.Second)
				continue
			}
			return nil, fmt.Errorf("pagespeed api: %w", lastErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
			bodyPreview = strings.TrimSpace(string(body))
			resp.Body.Close()
			if attempt < 2 {
				time.Sleep(2 * time.Second)
				continue
			}
			return nil, fmt.Errorf("pagespeed api returned %d: %s", resp.StatusCode, bodyPreview)
		}

		break
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("pagespeed api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	result := &PageSpeedResult{}

	// Extract performance score
	if lh, ok := raw["lighthouseResult"].(map[string]any); ok {
		if cats, ok := lh["categories"].(map[string]any); ok {
			if perf, ok := cats["performance"].(map[string]any); ok {
				if score, ok := perf["score"].(float64); ok {
					result.Score = int(score * 100)
				}
			}
		}
		if audits, ok := lh["audits"].(map[string]any); ok {
			result.FCP = extractAuditDisplay(audits, "first-contentful-paint")
			result.LCP = extractAuditDisplay(audits, "largest-contentful-paint")
			result.CLS = extractAuditDisplay(audits, "cumulative-layout-shift")
			result.SpeedIndex = extractAuditDisplay(audits, "speed-index")
			result.Interactive = extractAuditDisplay(audits, "interactive")
		}
	}

	// Save score to DB
	execErr := e.db.UpdatePageSpeedScore(ctx, placeID, result.Score)
	if execErr != nil {
		return result, fmt.Errorf("saved score but failed db update: %w", execErr)
	}

	return result, nil
}

func extractAuditDisplay(audits map[string]any, key string) string {
	if audit, ok := audits[key].(map[string]any); ok {
		if display, ok := audit["displayValue"].(string); ok {
			return display
		}
	}
	return ""
}

// ContactResult holds extracted contact information.
type ContactResult struct {
	Emails       []string `json:"emails"`
	Phones       []string `json:"phones"`
	Names        []string `json:"names"`
	LinkedInURLs []string `json:"linkedin_urls"`
}

var (
	emailExtractRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneExtractRegex = regexp.MustCompile(`(?:(?:\+|00)\d{1,3}[\s.-]?)?\(?\d{2,4}\)?[\s.-]?\d{3,4}[\s.-]?\d{3,4}`)
)

// ExtractContacts scrapes the lead's website for contact information.
func (e *Enricher) ExtractContacts(ctx context.Context, placeID string) (*ContactResult, error) {
	lead, err := e.db.GetLead(ctx, placeID)
	if err != nil {
		return nil, err
	}

	result := &ContactResult{}

	// Start with existing GMaps data
	if lead.Phone != "" {
		result.Phones = append(result.Phones, lead.Phone)
	}
	if len(lead.Emails) > 0 {
		result.Emails = append(result.Emails, lead.Emails...)
	}

	if lead.Website == "" {
		return result, nil
	}

	url := lead.Website
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	// Scrape the main page + common contact pages
	pages := []string{url}
	for _, path := range []string{"/contact", "/about", "/contact-us", "/about-us", "/team"} {
		pages = append(pages, strings.TrimRight(url, "/")+path)
	}

	seenEmails := make(map[string]bool)
	seenPhones := make(map[string]bool)

	for _, email := range result.Emails {
		seenEmails[email] = true
	}

	for _, pageURL := range pages {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		resp, err := e.httpClient.Do(req)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
		resp.Body.Close()

		if err != nil {
			continue
		}

		bodyStr := string(body)

		// Extract emails
		emails := emailExtractRegex.FindAllString(bodyStr, -1)
		for _, email := range emails {
			email = strings.ToLower(email)
			// Skip common false positives
			if strings.Contains(email, "example.com") ||
				strings.Contains(email, "sentry.io") ||
				strings.Contains(email, "wixpress") ||
				strings.Contains(email, ".png") ||
				strings.Contains(email, ".jpg") {
				continue
			}
			if !seenEmails[email] {
				seenEmails[email] = true
				result.Emails = append(result.Emails, email)
			}
		}

		// Extract phones
		phones := phoneExtractRegex.FindAllString(bodyStr, -1)
		for _, phone := range phones {
			phone = strings.TrimSpace(phone)
			digits := countDigits(phone)
			if digits >= 7 && digits <= 15 && !seenPhones[phone] {
				seenPhones[phone] = true
				result.Phones = append(result.Phones, phone)
			}
		}

		// Extract LinkedIn URLs
		linkedInRegex := regexp.MustCompile(`https?://(?:www\.)?linkedin\.com/in/[a-zA-Z0-9\-]+`)
		linkedIns := linkedInRegex.FindAllString(bodyStr, -1)
		result.LinkedInURLs = append(result.LinkedInURLs, linkedIns...)
	}

	// Update emails in DB if we found new ones
	if len(result.Emails) > 0 {
		hasValid := false
		for _, email := range result.Emails {
			parts := strings.Split(email, "@")
			if len(parts) == 2 {
				mx, err := net.LookupMX(parts[1])
				if err == nil && len(mx) > 0 {
					hasValid = true
					break
				}
			}
		}

		_ = e.db.UpdateEmails(ctx, placeID, result.Emails, hasValid)
	}

	return result, nil
}

func countDigits(s string) int {
	count := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			count++
		}
	}
	return count
}

// ScreenshotResult holds screenshot data.
type ScreenshotResult struct {
	URL       string `json:"url"`
	Timestamp string `json:"timestamp"`
}

// TakeScreenshot captures a website screenshot using a free API.
func (e *Enricher) TakeScreenshot(ctx context.Context, placeID string) (*ScreenshotResult, error) {
	lead, err := e.db.GetLead(ctx, placeID)
	if err != nil {
		return nil, err
	}

	if lead.Website == "" {
		return nil, fmt.Errorf("lead has no website")
	}

	url := lead.Website
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	// Use a free screenshot API
	screenshotURL := fmt.Sprintf(
		"https://image.thum.io/get/width/1280/crop/720/noanimate/%s",
		url,
	)

	return &ScreenshotResult{
		URL:       screenshotURL,
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// GeneratePitchRequest holds the pitch generation parameters.
type GeneratePitchRequest struct {
	PlaceID  string `json:"place_id"`
	Persona  string `json:"persona"`
	APIKey   string `json:"api_key"`
	Provider string `json:"provider"` // "gemini" or "openai"
}

// GeneratePitchPrompt builds a pitch prompt from lead data (client-side API call).
func (e *Enricher) GeneratePitchPrompt(ctx context.Context, placeID, persona, apiKey, provider, modelName string) (string, error) {
	lead, err := e.db.GetLead(ctx, placeID)
	if err != nil {
		return "", err
	}

	persona = strings.TrimSpace(persona)
	if persona == "" {
		persona = "marketing consultant"
	}

	issues := buildLeadIssues(lead)

	techStr := "unknown"
	if len(lead.TechStack) > 0 {
		techStr = strings.Join(lead.TechStack, ", ")
	}

	website := strings.TrimSpace(lead.Website)
	if website == "" {
		website = "No website listed"
	}

	businessName := strings.TrimSpace(lead.Title)
	if businessName == "" {
		businessName = "this business"
	}

	location := strings.Trim(strings.Join([]string{strings.TrimSpace(lead.City), strings.TrimSpace(lead.Country)}, ", "), ", ")
	if location == "" {
		location = "Unknown location"
	}

	category := strings.TrimSpace(lead.Category)
	if category == "" {
		category = "Local business"
	}

	// Fetch market leaders (competitors in same city + category with high ratings)
	competitorSection := e.buildCompetitorAnalysis(ctx, lead)
	if competitorSection == "" {
		competitorSection = "Competitive Analysis: Not enough competitor data available for this category and location."
	}

	prompt := fmt.Sprintf(`You are an expert %s at a digital growth agency.

Write one highly personalized outreach email that feels human and specific, not generic.

Lead Profile:
- Business: %s
- Category: %s
- Location: %s
- Website: %s
- Google Rating: %.1f stars (%d reviews)
- Detected Tech Stack: %s

Top Opportunities:
- %s

%s

Rules:
- Tone: warm, confident, and consultative (never pushy)
- Mention exactly 1 compliment and 1-2 concrete growth gaps from the data above
- If competitor insight exists, include one tactful benchmark in one short sentence
- Offer one low-friction next step (free audit, 15-minute teardown, or quick win plan)
- Keep total body length between 90 and 130 words
- Avoid buzzwords and avoid making unverifiable claims
- Plain text only (no markdown, no bullet points)

Return exactly this format:
Subject: <short subject line under 8 words>
Email: <single-paragraph email body>
`,
		persona,
		businessName,
		category,
		location,
		website,
		lead.ReviewRating, lead.ReviewCount,
		techStr,
		strings.Join(issues, "\n- "),
		competitorSection,
	)

	if provider == "" {
		provider = "gemini"
	}
	provider = strings.ToLower(strings.TrimSpace(provider))

	// Local mode generates a deterministic pitch without any model/API dependency.
	if provider == "local" || strings.TrimSpace(apiKey) == "" {
		return generateLocalPitch(lead, issues, competitorSection), nil
	}

	switch provider {
	case "gemini":
		return e.callGemini(ctx, prompt, apiKey, modelName)
	case "anthropic":
		return e.callAnthropic(ctx, prompt, apiKey, modelName)
	case "cohere":
		return e.callCohere(ctx, prompt, apiKey, modelName)
	default:
		return e.callOpenAICompatible(ctx, prompt, apiKey, provider, modelName)
	}
}

func generateLocalPitch(lead *Lead, issues []string, competitorSection string) string {
	businessName := strings.TrimSpace(lead.Title)
	if businessName == "" {
		businessName = "your business"
	}

	greeting := businessName
	if businessName == "your business" {
		greeting = "there"
	}

	location := strings.TrimSpace(lead.City)
	if location == "" {
		location = "your area"
	}

	compliment := fmt.Sprintf("I like how %s is positioned in %s.", businessName, location)
	if lead.ReviewRating >= 4.2 && lead.ReviewCount > 0 {
		compliment = fmt.Sprintf("Your %.1f-star rating across %d reviews stands out.", lead.ReviewRating, lead.ReviewCount)
	}

	opportunity := "you can likely improve lead capture and conversion from local search traffic"
	if len(issues) > 0 {
		opportunity = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(issues[0]), "."))
	}

	benchmark := ""
	if insight := firstCompetitorInsight(competitorSection); insight != "" {
		benchmark = " I also noticed " + insight + "."
	}

	subject := fmt.Sprintf("Quick idea for %s", businessName)
	body := fmt.Sprintf("Hi %s, %s From a quick review, %s. I can share a short plan with 2 practical fixes you can apply this month to improve calls and form leads.%s If you are open, I can send a free 10-point audit and walk you through it in 15 minutes.", greeting, compliment, opportunity, benchmark)

	return fmt.Sprintf("Subject: %s\nEmail: %s", subject, body)
}

func firstCompetitorInsight(section string) string {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") {
			insight := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			insight = strings.ReplaceAll(insight, "\"", "")
			insight = strings.TrimSpace(strings.TrimSuffix(insight, "."))
			if insight != "" {
				return insight
			}
		}
	}
	return ""
}

func buildLeadIssues(lead *Lead) []string {
	var issues []string
	for _, tag := range lead.ServiceTags {
		switch tag {
		case "No Website":
			issues = append(issues, "No website is listed, which limits discoverability and conversions")
		case "No SSL":
			issues = append(issues, "Website is missing SSL/HTTPS trust signals")
		case "No Analytics":
			issues = append(issues, "No analytics setup detected, so conversion tracking is likely missing")
		case "Missing Pixel":
			issues = append(issues, "No Meta/Facebook pixel detected for retargeting")
		case "Needs SEO":
			issues = append(issues, "Core SEO elements are weak (for example, missing key metadata)")
		case "Low Rating":
			issues = append(issues, fmt.Sprintf("Average rating is %.1f, which may reduce inbound trust", lead.ReviewRating))
		case "Few Reviews":
			issues = append(issues, fmt.Sprintf("Only %d reviews, leaving social proof lower than local leaders", lead.ReviewCount))
		case "Unclaimed GMB":
			issues = append(issues, "Google Business Profile may be unclaimed, reducing local search control")
		case "Slow Website":
			issues = append(issues, "Site speed appears slow, which can hurt rankings and lead quality")
		}
	}

	if len(issues) == 0 {
		issues = append(issues, "Opportunity to strengthen local lead capture and conversion flow")
	}

	return issues
}

// buildCompetitorAnalysis fetches market leaders and builds competitive insights.
func (e *Enricher) buildCompetitorAnalysis(ctx context.Context, lead *Lead) string {
	if lead.City == "" {
		return ""
	}

	// Fetch top competitors in same city with rating >= 4.5
	competitors, err := e.db.GetCompetitorsByCategory(ctx, lead.City, lead.Category, lead.PlaceID, 4.0)
	if err != nil || len(competitors) == 0 {
		return ""
	}

	var insights []string

	// Find the market leader (highest rated with most reviews)
	marketLeader := competitors[0]

	// Review comparison
	if marketLeader.ReviewCount > lead.ReviewCount*2 {
		if lead.ReviewCount == 0 {
			insights = append(insights, fmt.Sprintf("Market leader \"%s\" has %d reviews while this business has none", marketLeader.Title, marketLeader.ReviewCount))
		} else {
			multiplier := float64(marketLeader.ReviewCount) / float64(lead.ReviewCount)
			insights = append(insights, fmt.Sprintf("Market leader \"%s\" has %.0fx more reviews (%d vs %d)", marketLeader.Title, multiplier, marketLeader.ReviewCount, lead.ReviewCount))
		}
	}

	// Rating comparison
	if marketLeader.ReviewRating > lead.ReviewRating+0.5 {
		insights = append(insights, fmt.Sprintf("\"%s\" has a %.1f rating vs this business's %.1f", marketLeader.Title, marketLeader.ReviewRating, lead.ReviewRating))
	}

	// PageSpeed comparison
	if marketLeader.PageSpeedScore != nil && lead.PageSpeedScore != nil {
		leaderSpeed := *marketLeader.PageSpeedScore
		leadSpeed := *lead.PageSpeedScore
		if leaderSpeed > leadSpeed+20 {
			insights = append(insights, fmt.Sprintf("Competitor's website is faster (PageSpeed %d vs %d)", leaderSpeed, leadSpeed))
		}
	}

	// SSL comparison
	if marketLeader.HasSSL != nil && lead.HasSSL != nil {
		if *marketLeader.HasSSL && !*lead.HasSSL {
			insights = append(insights, "Top competitors have SSL/HTTPS while this business doesn't")
		}
	}

	// Analytics comparison
	if marketLeader.HasAnalytics != nil && lead.HasAnalytics != nil {
		if *marketLeader.HasAnalytics && !*lead.HasAnalytics {
			insights = append(insights, "Market leaders use Google Analytics for tracking while this business doesn't")
		}
	}

	// Website presence comparison
	if marketLeader.Website != "" && lead.Website == "" {
		insights = append(insights, fmt.Sprintf("Competitor \"%s\" has a professional website while this business has no online presence", marketLeader.Title))
	}

	if len(insights) == 0 {
		return ""
	}

	return fmt.Sprintf("**Competitive Analysis (Market Leaders in %s - %s):**\n- %s",
		lead.City, lead.Category, strings.Join(insights, "\n- "))
}

func (e *Enricher) callGemini(ctx context.Context, prompt, apiKey, modelName string) (string, error) {
	if modelName == "" {
		modelName = "gemini-1.5-flash"
	}
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + modelName + ":generateContent?key=" + apiKey

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": prompt},
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyStr, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini api error: %d - %s", resp.StatusCode, string(bodyStr))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no content returned from gemini")
}

func (e *Enricher) callOpenAICompatible(ctx context.Context, prompt, apiKey, provider, customModel string) (string, error) {
	var url, model string
	switch provider {
	case "openai":
		url, model = "https://api.openai.com/v1/chat/completions", "gpt-4o-mini"
	case "groq":
		url, model = "https://api.groq.com/openai/v1/chat/completions", "llama3-8b-8192"
	case "mistral":
		url, model = "https://api.mistral.ai/v1/chat/completions", "mistral-large-latest"
	case "together":
		url, model = "https://api.together.xyz/v1/chat/completions", "meta-llama/Llama-3-70b-chat-hf"
	case "perplexity":
		url, model = "https://api.perplexity.ai/chat/completions", "llama-3-sonar-large-32k-chat"
	case "replicate":
		url, model = "https://openai-proxy.replicate.com/v1/chat/completions", "meta/llama-3-70b-instruct"
	case "huggingface":
		url, model = "https://api-inference.huggingface.co/v1/chat/completions", "meta-llama/Meta-Llama-3-8B-Instruct"
	case "ollama":
		if strings.HasPrefix(apiKey, "http") {
			url = strings.TrimSuffix(apiKey, "/") + "/v1/chat/completions"
		} else {
			url = "http://localhost:11434/v1/chat/completions"
		}
		model = "qwen3-coder:480b-cloud"
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	if customModel != "" {
		model = customModel
	}

	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if provider != "ollama" || !strings.HasPrefix(apiKey, "http") {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyStr, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%s api error: %d - %s", provider, resp.StatusCode, string(bodyStr))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("no content returned from %s", provider)
}

func (e *Enricher) callAnthropic(ctx context.Context, prompt, apiKey, modelName string) (string, error) {
	if modelName == "" {
		modelName = "claude-3-5-sonnet-20241022"
	}
	url := "https://api.anthropic.com/v1/messages"
	payload := map[string]any{
		"model":      modelName,
		"max_tokens": 1024,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyStr, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic api error: %d - %s", resp.StatusCode, string(bodyStr))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", fmt.Errorf("no content returned from anthropic")
}

func (e *Enricher) callCohere(ctx context.Context, prompt, apiKey, modelName string) (string, error) {
	if modelName == "" {
		modelName = "command-r-plus"
	}
	url := "https://api.cohere.com/v1/chat"
	payload := map[string]any{
		"model":   modelName,
		"message": prompt,
	}
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyStr, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("cohere api error: %d - %s", resp.StatusCode, string(bodyStr))
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Text, nil
}
