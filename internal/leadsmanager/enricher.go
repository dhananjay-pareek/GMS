package leadsmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	wappalyzer "github.com/projectdiscovery/wappalyzergo"
)

// Enricher handles on-demand enrichment tasks for leads.
type Enricher struct {
	db         *DB
	httpClient *http.Client
	wappalyze  *wappalyzer.Wappalyze
}

// NewEnricher creates a new enrichment engine.
func NewEnricher(db *DB) (*Enricher, error) {
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
	_, execErr := e.db.pool.Exec(ctx,
		"UPDATE gmaps_leads SET tech_stack = $1 WHERE place_id = $2",
		techs, placeID,
	)
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

	url := lead.Website
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	apiURL := fmt.Sprintf(
		"https://www.googleapis.com/pagespeedonline/v5/runPagespeed?url=%s&strategy=mobile",
		url,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pagespeed api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pagespeed api returned %d", resp.StatusCode)
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
	_, execErr := e.db.pool.Exec(ctx,
		"UPDATE gmaps_leads SET page_speed_score = $1 WHERE place_id = $2",
		result.Score, placeID,
	)
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

		_, _ = e.db.pool.Exec(ctx,
			"UPDATE gmaps_leads SET emails = $1, is_email_valid = $2 WHERE place_id = $3",
			result.Emails, hasValid, placeID,
		)
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
func (e *Enricher) GeneratePitchPrompt(ctx context.Context, placeID, persona string) (string, error) {
	lead, err := e.db.GetLead(ctx, placeID)
	if err != nil {
		return "", err
	}

	var issues []string
	for _, tag := range lead.ServiceTags {
		switch tag {
		case "No Website":
			issues = append(issues, "They don't have a website at all")
		case "No SSL":
			issues = append(issues, "Their website lacks SSL/HTTPS security")
		case "No Analytics":
			issues = append(issues, "They have no Google Analytics tracking")
		case "Missing Pixel":
			issues = append(issues, "They're missing Facebook Pixel for ad tracking")
		case "Needs SEO":
			issues = append(issues, "Their website has SEO issues (missing H1 tags or meta descriptions)")
		case "Low Rating":
			issues = append(issues, fmt.Sprintf("They have a low Google rating of %.1f stars", lead.ReviewRating))
		case "Few Reviews":
			issues = append(issues, fmt.Sprintf("They only have %d Google reviews", lead.ReviewCount))
		case "Unclaimed GMB":
			issues = append(issues, "Their Google Business Profile appears to be unclaimed")
		case "Slow Website":
			issues = append(issues, "Their website is slow (poor PageSpeed score)")
		}
	}

	techStr := "unknown"
	if len(lead.TechStack) > 0 {
		techStr = strings.Join(lead.TechStack, ", ")
	}

	prompt := fmt.Sprintf(`You are a %s at a digital marketing agency. Write a personalized cold email pitch for this business:

**Business:** %s
**Category:** %s  
**Location:** %s, %s
**Website:** %s
**Google Rating:** %.1f stars (%d reviews)
**Tech Stack:** %s

**Issues Found:**
%s

Instructions:
- Be conversational and NOT salesy
- Lead with a specific compliment about their business
- Mention 1-2 specific issues you noticed (be tactful, not critical)
- Offer a free audit or consultation
- Keep it under 150 words
- Include a clear call to action
- Sign off professionally`,
		persona,
		lead.Title,
		lead.Category,
		lead.City, lead.Country,
		lead.Website,
		lead.ReviewRating, lead.ReviewCount,
		techStr,
		strings.Join(issues, "\n- "),
	)

	return prompt, nil
}
