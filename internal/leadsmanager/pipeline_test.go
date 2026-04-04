package leadsmanager

import (
	"testing"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/stretchr/testify/require"
)

func TestProcessEntry_BasicFields(t *testing.T) {
	entry := gmaps.Entry{
		PlaceID:  "ChIJtest123",
		Title:    "Test Business",
		Category: "Restaurant",
		Address:  "123 Main St",
		Phone:    "+1 555-1234567",
		WebSite:  "",
		CompleteAddress: gmaps.Address{
			City:    "New York",
			State:   "NY",
			Country: "US",
		},
		ReviewCount:  50,
		ReviewRating: 4.5,
		Owner: gmaps.Owner{
			ID:   "owner123",
			Name: "John Doe",
		},
	}

	lead := ProcessEntry(entry)

	require.Equal(t, "ChIJtest123", lead.PlaceID)
	require.Equal(t, "Test Business", lead.Title)
	require.Equal(t, "Restaurant", lead.Category)
	require.Equal(t, "New York", lead.City)
	require.Equal(t, "NY", lead.State)
	require.Equal(t, "US", lead.Country)
	require.True(t, lead.GmbClaimed)
	require.Equal(t, "John Doe", lead.OwnerName)
}

func TestProcessEntry_ValidEmail(t *testing.T) {
	valid := validateEmails([]string{"test@example.com"})
	require.True(t, valid)
}

func TestProcessEntry_InvalidEmail(t *testing.T) {
	invalid := validateEmails([]string{"notanemail", "", "missing@"})
	require.False(t, invalid)
}

func TestProcessEntry_EmptyEmails(t *testing.T) {
	result := validateEmails([]string{})
	require.False(t, result)
}

func TestProcessEntry_PhoneValid(t *testing.T) {
	tests := []struct {
		phone string
		valid bool
	}{
		{"+1 555-1234567", true},
		{"(212) 555-1234", true},
		{"+44 20 7946 0958", true},
		{"12345", false},
		{"", false},
		{"abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.phone, func(t *testing.T) {
			result := validatePhone(tt.phone)
			require.Equal(t, tt.valid, result, "phone: %s", tt.phone)
		})
	}
}

func TestProcessEntry_GmbClaimed(t *testing.T) {
	// With owner ID = claimed
	entry := gmaps.Entry{
		PlaceID:  "test1",
		Title:    "Test",
		Category: "Shop",
		Owner:    gmaps.Owner{ID: "owner123"},
	}
	lead := ProcessEntry(entry)
	require.True(t, lead.GmbClaimed)

	// Without owner ID = unclaimed
	entry2 := gmaps.Entry{
		PlaceID:  "test2",
		Title:    "Test2",
		Category: "Shop",
		Owner:    gmaps.Owner{},
	}
	lead2 := ProcessEntry(entry2)
	require.False(t, lead2.GmbClaimed)
}

func TestProcessEntry_ServiceTags_NoWebsite(t *testing.T) {
	entry := gmaps.Entry{
		PlaceID:      "test-no-website",
		Title:        "No Website Co",
		Category:     "Cafe",
		WebSite:      "",
		ReviewCount:  5,
		ReviewRating: 3.5,
		Owner:        gmaps.Owner{},
	}

	lead := ProcessEntry(entry)

	require.Contains(t, lead.ServiceTags, "No Website")
}

func TestProcessEntry_ServiceTags_LowRating(t *testing.T) {
	entry := gmaps.Entry{
		PlaceID:      "test-low-rating",
		Title:        "Bad Reviews Inc",
		Category:     "Store",
		ReviewCount:  100,
		ReviewRating: 2.5,
		Owner:        gmaps.Owner{ID: "owner"},
	}

	lead := ProcessEntry(entry)

	require.Contains(t, lead.ServiceTags, "Low Rating")
}

func TestProcessEntry_ServiceTags_FewReviews(t *testing.T) {
	entry := gmaps.Entry{
		PlaceID:      "test-few-reviews",
		Title:        "New Place",
		Category:     "Restaurant",
		ReviewCount:  3,
		ReviewRating: 5.0,
		Owner:        gmaps.Owner{ID: "owner"},
	}

	lead := ProcessEntry(entry)

	require.Contains(t, lead.ServiceTags, "Few Reviews")
}

func TestProcessEntry_ServiceTags_UnclaimedGMB(t *testing.T) {
	entry := gmaps.Entry{
		PlaceID:  "test-unclaimed",
		Title:    "Unclaimed Biz",
		Category: "Shop",
		Owner:    gmaps.Owner{},
	}

	lead := ProcessEntry(entry)

	require.Contains(t, lead.ServiceTags, "Unclaimed GMB")
}

func TestContainsTag(t *testing.T) {
	tags := []string{"No SSL", "Low Rating", "Needs SEO"}
	require.True(t, containsTag(tags, "Low Rating"))
	require.False(t, containsTag(tags, "No Website"))
}

func TestFormatRating(t *testing.T) {
	require.Equal(t, "4.5", FormatRating(4.5))
	require.Equal(t, "N/A", FormatRating(0))
	require.Equal(t, "3.0", FormatRating(3.0))
}
