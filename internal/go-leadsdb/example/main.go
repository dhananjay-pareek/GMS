package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dhananjay-pareek/google-maps-scraper/internal/go-leadsdb"
)

func main() {
	apiKey := os.Getenv("LEADSDB_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "LEADSDB_API_KEY environment variable is required")
		os.Exit(1)
	}

	client := leadsdb.New(apiKey)
	ctx := context.Background()

	// Create a new lead with all fields
	lead, err := client.Create(ctx, &leadsdb.Lead{
		Name:        "Acme Corporation",
		Source:      "website",
		Description: "Leading provider of innovative solutions",
		Address:     "123 Main Street",
		City:        "San Francisco",
		State:       "CA",
		Country:     "USA",
		PostalCode:  "94105",
		Latitude:    leadsdb.Ptr(37.7749),
		Longitude:   leadsdb.Ptr(-122.4194),
		Phone:       "+1-555-123-4567",
		Email:       "contact@acme.com",
		Website:     "https://acme.com",
		Rating:      leadsdb.Ptr(4.5),
		ReviewCount: leadsdb.Ptr(128),
		Category:    "Technology",
		Tags:        []string{"enterprise", "saas", "b2b"},
		SourceID:    "acme-001",
		LogoURL:     "https://acme.com/logo.png",
		Attributes: []leadsdb.Attribute{
			leadsdb.TextAttr("industry", "Software"),
			leadsdb.NumberAttr("employees", 500),
			leadsdb.BoolAttr("verified", true),
			leadsdb.ListAttr("products", []string{"CRM", "ERP", "Analytics"}),
			leadsdb.ObjectAttr("social", map[string]any{
				"linkedin": "https://linkedin.com/company/acme",
				"twitter":  "https://twitter.com/acme",
			}),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating lead: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created lead: %s (%s)\n", lead.Name, lead.ID)

	// Get the lead by ID
	lead, err = client.Get(ctx, lead.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting lead: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Retrieved lead: %s (%s)\n", lead.Name, lead.ID)

	// Update the lead
	lead, err = client.Update(ctx, lead.ID, &leadsdb.UpdateLeadInput{
		Rating: leadsdb.Ptr(4.8),
		City:   leadsdb.Ptr("New York"),
		Tags:   []string{"enterprise", "saas", "b2b", "updated"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating lead: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Updated lead: %s (rating: %.1f, city: %s)\n", lead.Name, *lead.Rating, lead.City)

	// Create a note for the lead
	note, err := client.CreateNote(ctx, lead.ID, "Initial contact made via website")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating note: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created note: %s\n", note.ID)

	// List notes for the lead
	notes, err := client.ListNotes(ctx, lead.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing notes: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Lead has %d note(s)\n", len(notes))

	// Update the note
	note, err = client.UpdateNote(ctx, note.ID, "Initial contact made via website - follow up scheduled")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating note: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Updated note: %s\n", note.Content)

	// Delete the note
	err = client.DeleteNote(ctx, note.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting note: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Deleted note")

	// Delete the lead
	err = client.Delete(ctx, lead.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting lead: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleted lead: %s\n", lead.ID)

	// Bulk create leads
	bulkResult, err := client.BulkCreate(ctx, []*leadsdb.Lead{
		{Name: "Lead 1", Source: "import", City: "Athens"},
		{Name: "Lead 2", Source: "import", City: "London"},
		{Name: "Lead 3", Source: "import", City: "Paris"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error bulk creating leads: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Bulk created: %d success, %d failed\n", bulkResult.Success, bulkResult.Failed)
	for _, created := range bulkResult.Created {
		fmt.Printf("  - %s (index %d)\n", created.ID, created.Index)
	}

	// Clean up bulk created leads
	for _, created := range bulkResult.Created {
		_ = client.Delete(ctx, created.ID)
	}

	fmt.Println("Cleaned up bulk created leads")

	// Clean up any existing leads with source 'channel-import' before testing
	for {
		existing, err := client.List(ctx, leadsdb.Source().Eq("channel-import"), leadsdb.Limit(100))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing existing leads: %v\n", err)
			os.Exit(1)
		}
		if len(existing.Leads) == 0 {
			break
		}
		for _, l := range existing.Leads {
			_ = client.Delete(ctx, l.ID)
		}
	}

	// Bulk create from channel
	leads := make(chan *leadsdb.Lead)

	go func() {
		defer close(leads)
		for i := range 10 {
			leads <- &leadsdb.Lead{
				Name:   fmt.Sprintf("Channel Lead %d", i),
				Source: "channel-import",
				City:   "Berlin",
			}
		}
	}()

	results, errs := client.BulkCreateFromChan(ctx, leads)

	var createdIDs []string

	go func() {
		for err := range errs {
			fmt.Fprintf(os.Stderr, "Channel error: %v\n", err)
		}
	}()

	for result := range results {
		fmt.Printf("Channel created: %s\n", result.ID)
		createdIDs = append(createdIDs, result.ID)
	}

	fmt.Printf("Channel bulk created: %d leads\n", len(createdIDs))

	// List leads with manual pagination
	fmt.Println("\nListing leads with manual pagination:")

	var allListed []leadsdb.Lead
	cursor := ""

	for {
		opts := []leadsdb.ListOption{
			leadsdb.Source().Eq("channel-import"),
			leadsdb.Sort(leadsdb.FieldName, leadsdb.Asc),
			leadsdb.Limit(3),
		}
		if cursor != "" {
			opts = append(opts, leadsdb.Cursor(cursor))
		}

		listResult, err := client.List(ctx, opts...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing leads: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("  Page: %d leads (has_more: %v)\n", listResult.Count, listResult.HasMore)
		for _, l := range listResult.Leads {
			fmt.Printf("    - %s (%s)\n", l.Name, l.City)
		}

		allListed = append(allListed, listResult.Leads...)

		if !listResult.HasMore {
			break
		}
		cursor = listResult.NextCursor
	}

	fmt.Printf("Total listed with manual pagination: %d leads\n", len(allListed))

	// List leads using iterator (handles pagination automatically)
	fmt.Println("\nListing leads in Berlin using iterator:")

	var iteratorListed []leadsdb.Lead
	for lead, err := range client.Iterator(ctx,
		leadsdb.City().Eq("Berlin"),
		leadsdb.Source().Eq("channel-import"),
		leadsdb.Sort(leadsdb.FieldName, leadsdb.Desc),
		leadsdb.Limit(4),
	) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error iterating leads: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  - %s (%s)\n", lead.Name, lead.City)
		iteratorListed = append(iteratorListed, *lead)
	}

	fmt.Printf("Total listed with iterator: %d leads\n", len(iteratorListed))

	// List leads using channel-based iterator
	fmt.Println("\nListing leads using channel-based iterator:")

	leadsChan, errsChan := client.IteratorChan(ctx,
		leadsdb.Source().Eq("channel-import"),
		leadsdb.Sort(leadsdb.FieldName, leadsdb.Asc),
		leadsdb.Limit(5),
	)

	go func() {
		for err := range errsChan {
			fmt.Fprintf(os.Stderr, "Channel iterator error: %v\n", err)
		}
	}()

	var chanListed []leadsdb.Lead
	for lead := range leadsChan {
		fmt.Printf("  - %s (%s)\n", lead.Name, lead.City)
		chanListed = append(chanListed, *lead)
	}

	fmt.Printf("Total listed with channel iterator: %d leads\n", len(chanListed))

	// Verify we got all the leads we created
	if len(allListed) != len(createdIDs) {
		fmt.Fprintf(os.Stderr, "ERROR: Expected %d leads, got %d\n", len(createdIDs), len(allListed))
		os.Exit(1)
	}

	listedIDs := make(map[string]bool)
	for _, l := range allListed {
		listedIDs[l.ID] = true
	}

	for _, id := range createdIDs {
		if !listedIDs[id] {
			fmt.Fprintf(os.Stderr, "ERROR: Created lead %s not found in list\n", id)
			os.Exit(1)
		}
	}

	fmt.Println("Verified: all created leads were listed correctly")

	// Clean up channel created leads
	for _, id := range createdIDs {
		_ = client.Delete(ctx, id)
	}

	fmt.Println("Cleaned up channel created leads")
}
