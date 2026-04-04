package leadsmanager

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gosom/google-maps-scraper/gmaps"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	phoneRegex = regexp.MustCompile(`^[\+]?[0-9\s\-\(\)]{7,20}$`)
)

// ProcessEntry converts a scraped gmaps.Entry into an enriched Lead.
func ProcessEntry(entry gmaps.Entry) Lead {
	lead := Lead{
		PlaceID:      entry.PlaceID,
		Title:        entry.Title,
		Category:     entry.Category,
		Categories:   entry.Categories,
		Address:      entry.Address,
		City:         entry.CompleteAddress.City,
		State:        entry.CompleteAddress.State,
		Country:      entry.CompleteAddress.Country,
		PostalCode:   entry.CompleteAddress.PostalCode,
		Phone:        entry.Phone,
		Emails:       entry.Emails,
		Website:      entry.WebSite,
		ReviewCount:  entry.ReviewCount,
		ReviewRating: entry.ReviewRating,
		Latitude:     entry.Latitude,
		Longitude:    entry.Longtitude,
		GmapsLink:    entry.Link,
		Cid:          entry.Cid,
		Status:       entry.Status,
		Description:  entry.Description,
		OwnerName:    entry.Owner.Name,
		OwnerID:      entry.Owner.ID,
		Thumbnail:    entry.Thumbnail,
		Timezone:     entry.Timezone,
		PriceRange:   entry.PriceRange,
		PlusCode:     entry.PlusCode,
	}

	if lead.Categories == nil {
		lead.Categories = []string{}
	}
	if lead.Emails == nil {
		lead.Emails = []string{}
	}

	// Validate emails
	lead.IsEmailValid = validateEmails(lead.Emails)

	// Validate phone
	lead.IsPhoneValid = validatePhone(lead.Phone)

	// GMB claimed check
	lead.GmbClaimed = lead.OwnerID != ""

	// Website-based enrichment (only if website exists)
	if lead.Website != "" {
		enrichFromWebsite(&lead)
	}

	// Generate service tags
	lead.ServiceTags = generateServiceTags(&lead)

	// Set social links as JSON
	lead.SocialLinks = "{}"

	return lead
}

// validateEmails checks if at least one email in the list is valid.
func validateEmails(emails []string) bool {
	for _, email := range emails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		if emailRegex.MatchString(email) {
			// Check MX record
			parts := strings.Split(email, "@")
			if len(parts) == 2 {
				mx, err := net.LookupMX(parts[1])
				if err == nil && len(mx) > 0 {
					return true
				}
				// Even without MX, the format is valid
				return true
			}
		}
	}
	return false
}

// validatePhone checks if the phone number looks valid.
func validatePhone(phone string) bool {
	if phone == "" {
		return false
	}
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '+' {
			return r
		}
		return -1
	}, phone)
	// Must have at least 7 digits
	digitCount := 0
	for _, c := range cleaned {
		if c >= '0' && c <= '9' {
			digitCount++
		}
	}
	return digitCount >= 7 && phoneRegex.MatchString(phone)
}

// enrichFromWebsite performs lightweight website analysis.
func enrichFromWebsite(lead *Lead) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := lead.Website
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	// SSL Check
	hasSSL := checkSSL(url)
	lead.HasSSL = &hasSSL

	// Fetch the page
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB limit
	if err != nil {
		return
	}

	bodyStr := string(body)

	// Analytics check
	hasAnalytics := strings.Contains(bodyStr, "gtag(") ||
		strings.Contains(bodyStr, "google-analytics.com") ||
		strings.Contains(bodyStr, "googletagmanager.com") ||
		strings.Contains(bodyStr, "ga('create")
	lead.HasAnalytics = &hasAnalytics

	// Facebook Pixel check
	hasFBPixel := strings.Contains(bodyStr, "fbq(") ||
		strings.Contains(bodyStr, "connect.facebook.net") ||
		strings.Contains(bodyStr, "facebook-jssdk")
	lead.HasFacebookPixel = &hasFBPixel

	// Parse HTML for meta/h1
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(bodyStr))
	if err != nil {
		return
	}

	// H1 check
	hasH1 := doc.Find("h1").Length() > 0
	lead.HasH1 = &hasH1

	// Meta description check
	metaDesc, _ := doc.Find("meta[name='description']").Attr("content")
	hasMetaDesc := metaDesc != ""
	lead.HasMetaDesc = &hasMetaDesc

	// Social presence
	socialLinks := make(map[string]string)
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		href = strings.ToLower(href)
		if strings.Contains(href, "facebook.com") {
			socialLinks["facebook"] = href
		} else if strings.Contains(href, "instagram.com") {
			socialLinks["instagram"] = href
		} else if strings.Contains(href, "linkedin.com") {
			socialLinks["linkedin"] = href
		} else if strings.Contains(href, "twitter.com") || strings.Contains(href, "x.com") {
			socialLinks["twitter"] = href
		} else if strings.Contains(href, "youtube.com") {
			socialLinks["youtube"] = href
		}
	})

	if len(socialLinks) > 0 {
		sl, _ := json.Marshal(socialLinks)
		lead.SocialLinks = string(sl)
	}
}

// checkSSL verifies if the website supports HTTPS.
func checkSSL(url string) bool {
	httpsURL := url
	if strings.HasPrefix(url, "http://") {
		httpsURL = strings.Replace(url, "http://", "https://", 1)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Head(httpsURL)
	if err != nil {
		return false
	}
	resp.Body.Close()

	return resp.TLS != nil || strings.HasPrefix(httpsURL, "https://")
}

// generateServiceTags creates agency-relevant tags based on lead weaknesses.
func generateServiceTags(lead *Lead) []string {
	var tags []string

	// No website at all
	if lead.Website == "" {
		tags = append(tags, "No Website")
	} else {
		// Website-specific checks (only if website exists)
		if lead.HasSSL != nil && !*lead.HasSSL {
			tags = append(tags, "No SSL")
		}
		if lead.HasAnalytics != nil && !*lead.HasAnalytics {
			tags = append(tags, "No Analytics")
		}
		if lead.HasFacebookPixel != nil && !*lead.HasFacebookPixel {
			tags = append(tags, "Missing Pixel")
		}
		if lead.HasH1 != nil && !*lead.HasH1 {
			tags = append(tags, "Needs SEO")
		}
		if lead.HasMetaDesc != nil && !*lead.HasMetaDesc {
			if !containsTag(tags, "Needs SEO") {
				tags = append(tags, "Needs SEO")
			}
		}
	}

	// Low rating
	if lead.ReviewRating > 0 && lead.ReviewRating < 4.0 {
		tags = append(tags, "Low Rating")
	}

	// Low reviews
	if lead.ReviewCount < 10 {
		tags = append(tags, "Few Reviews")
	}

	// GMB not claimed
	if !lead.GmbClaimed {
		tags = append(tags, "Unclaimed GMB")
	}

	// No email found
	if len(lead.Emails) == 0 || !lead.IsEmailValid {
		tags = append(tags, "No Email")
	}

	// No phone
	if lead.Phone == "" || !lead.IsPhoneValid {
		tags = append(tags, "No Phone")
	}

	// PageSpeed
	if lead.PageSpeedScore != nil && *lead.PageSpeedScore < 50 {
		tags = append(tags, "Slow Website")
	}

	if tags == nil {
		tags = []string{}
	}

	return tags
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// FormatRating returns a display string for the review rating.
func FormatRating(rating float64) string {
	if rating == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.1f", rating)
}
