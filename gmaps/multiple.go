package gmaps

import (
	"encoding/json"
	"fmt"
	"strings"

	olc "github.com/google/open-location-code/go"
)

func ParseSearchResults(raw []byte) ([]*Entry, error) {
	var data []any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty JSON data")
	}

	container, ok := data[0].([]any)
	if !ok || len(container) == 0 {
		return nil, fmt.Errorf("invalid business list structure")
	}

	items := getNthElementAndCast[[]any](container, 1)
	if len(items) < 2 {
		return nil, fmt.Errorf("empty business list")
	}

	entries := make([]*Entry, 0, len(items)-1)

	for i := 1; i < len(items); i++ {
		arr, ok := items[i].([]any)
		if !ok {
			continue
		}

		business := getNthElementAndCast[[]any](arr, 14)

		var entry Entry

		entry.ID = getNthElementAndCast[string](business, 0)
		entry.Title = getNthElementAndCast[string](business, 11)
		entry.Categories = toStringSlice(getNthElementAndCast[[]any](business, 13))
		entry.WebSite = extractWebsite(business)

		// Set primary category from categories list
		if len(entry.Categories) > 0 {
			entry.Category = entry.Categories[0]
		}

		entry.ReviewRating = getNthElementAndCast[float64](business, 4, 7)
		entry.ReviewCount = int(getNthElementAndCast[float64](business, 4, 8))

		fullAddress := getNthElementAndCast[[]any](business, 2)

		entry.Address = func() string {
			sb := strings.Builder{}

			for i, part := range fullAddress {
				if i > 0 {
					sb.WriteString(", ")
				}

				sb.WriteString(fmt.Sprintf("%v", part))
			}

			return sb.String()
		}()

		// Parse address components from full address
		entry.CompleteAddress = parseAddressComponents(entry.Address)

		entry.Latitude = getNthElementAndCast[float64](business, 9, 2)
		entry.Longtitude = getNthElementAndCast[float64](business, 9, 3)
		entry.Phone = strings.ReplaceAll(extractPhone(business), " ", "")
		entry.OpenHours = getHours(business)
		entry.Status = getNthElementAndCast[string](business, 34, 4, 4)
		entry.Timezone = getNthElementAndCast[string](business, 30)
		entry.DataID = getNthElementAndCast[string](business, 10)
		entry.PlaceID = getNthElementAndCast[string](business, 78)
		entry.Link = getNthElementAndCast[string](business, 27)

		// Generate Google Maps link if not present
		if entry.Link == "" && entry.PlaceID != "" {
			entry.Link = "https://www.google.com/maps/place/?q=place_id:" + entry.PlaceID
		}

		// Generate CID from DataID if available
		if entry.Cid == "" && entry.DataID != "" {
			entry.Cid = extractCidFromDataID(entry.DataID)
		}

		entry.PlusCode = olc.Encode(entry.Latitude, entry.Longtitude, 10)

		entries = append(entries, &entry)
	}

	return entries, nil
}

// parseAddressComponents attempts to parse address string into components
func parseAddressComponents(address string) Address {
	parts := strings.Split(address, ", ")
	addr := Address{}

	if len(parts) == 0 {
		return addr
	}

	// Last part is usually country
	if len(parts) >= 1 {
		addr.Country = parts[len(parts)-1]
	}

	// For Australian addresses: "47 Carlotta St, Artarmon NSW 2064, Australia"
	// Format is typically: Street, City State PostCode, Country
	if len(parts) >= 2 {
		cityStatePostal := parts[len(parts)-2]
		// Try to extract state and postal code (e.g., "Artarmon NSW 2064")
		cityParts := strings.Fields(cityStatePostal)
		if len(cityParts) >= 1 {
			// Check if last part is postal code (numeric)
			lastIdx := len(cityParts) - 1
			if isPostalCode(cityParts[lastIdx]) {
				addr.PostalCode = cityParts[lastIdx]
				lastIdx--
			}
			// Check if second-to-last is a state abbreviation
			if lastIdx >= 1 && isStateAbbrev(cityParts[lastIdx]) {
				addr.State = cityParts[lastIdx]
				lastIdx--
			}
			// Rest is city
			if lastIdx >= 0 {
				addr.City = strings.Join(cityParts[:lastIdx+1], " ")
			}
		}
	}

	// First part is street
	if len(parts) >= 1 {
		addr.Street = parts[0]
	}

	return addr
}

func isPostalCode(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) >= 3 && len(s) <= 10
}

func isStateAbbrev(s string) bool {
	// Common Australian state abbreviations
	states := map[string]bool{
		"NSW": true, "VIC": true, "QLD": true, "SA": true,
		"WA": true, "TAS": true, "NT": true, "ACT": true,
		// US states (some common ones)
		"CA": true, "NY": true, "TX": true, "FL": true,
	}
	return states[strings.ToUpper(s)]
}

func extractCidFromDataID(dataID string) string {
	// DataID format: "0x6b12af94dd6ab4b7:0x6d5eebf740b4b010"
	// CID is the hex part after the colon
	parts := strings.Split(dataID, ":")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func toStringSlice(arr []any) []string {
	ans := make([]string, 0, len(arr))
	for _, v := range arr {
		ans = append(ans, fmt.Sprintf("%v", v))
	}

	return ans
}
