# go-leadsdb

[![GoDoc](https://godoc.org/github.com/gosom/go-leadsdb?status.svg)](https://godoc.org/github.com/gosom/go-leadsdb)
[![Go Report Card](https://goreportcard.com/badge/github.com/gosom/go-leadsdb)](https://goreportcard.com/report/github.com/gosom/go-leadsdb)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Go SDK for the [LeadsDB API](https://getleadsdb.com).

## Installation

```bash
go get github.com/gosom/go-leadsdb
```

Requires Go 1.23+.

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/gosom/go-leadsdb"
)

func main() {
    client := leadsdb.New("your-api-key")
    ctx := context.Background()

    // Create a lead
    lead, err := client.Create(ctx, &leadsdb.Lead{
        Name:    "Acme Corporation",
        Source:  "website",
        City:    "San Francisco",
        Email:   "contact@acme.com",
        Rating:  leadsdb.Ptr(4.5),
        Tags:    []string{"enterprise", "saas"},
    })
    if err != nil {
        panic(err)
    }

    fmt.Printf("Created: %s (%s)\n", lead.Name, lead.ID)
}
```

## Client Options

```go
client := leadsdb.New(apiKey,
    leadsdb.WithBaseURL("https://custom.api.com"),
    leadsdb.WithTimeout(30 * time.Second),
    leadsdb.WithMaxRetries(5),
    leadsdb.WithHTTPClient(customClient),
)
```

## CRUD Operations

### Create

```go
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
```

### Get

```go
lead, err := client.Get(ctx, "lead-id")
```

### Update

Use `UpdateLeadInput` with pointer fields for partial updates:

```go
lead, err := client.Update(ctx, "lead-id", &leadsdb.UpdateLeadInput{
    Rating: leadsdb.Ptr(4.8),
    City:   leadsdb.Ptr("New York"),
    Tags:   []string{"enterprise", "saas", "b2b", "updated"},
})
```

### Delete

```go
err := client.Delete(ctx, "lead-id")
```

## Listing Leads

### Basic List

```go
result, err := client.List(ctx,
    leadsdb.City().Eq("Berlin"),
    leadsdb.Rating().Gte(4.0),
    leadsdb.Sort(leadsdb.FieldName, leadsdb.Asc),
    leadsdb.Limit(20),
)

for _, lead := range result.Leads {
    fmt.Printf("%s (%s)\n", lead.Name, lead.City)
}

// Pagination
if result.HasMore {
    nextResult, err := client.List(ctx,
        leadsdb.City().Eq("Berlin"),
        leadsdb.Cursor(result.NextCursor),
    )
}
```

### Iterator (Automatic Pagination)

```go
for lead, err := range client.Iterator(ctx,
    leadsdb.City().Eq("Berlin"),
    leadsdb.Sort(leadsdb.FieldName, leadsdb.Desc),
) {
    if err != nil {
        panic(err)
    }
    fmt.Printf("%s\n", lead.Name)
}
```

### Channel-Based Iterator

```go
leads, errs := client.IteratorChan(ctx,
    leadsdb.Source().Eq("import"),
    leadsdb.Limit(100),
)

go func() {
    for err := range errs {
        log.Printf("Error: %v", err)
    }
}()

for lead := range leads {
    fmt.Printf("%s\n", lead.Name)
}
```

## Filters

All filters default to AND logic. Use `Or()` for OR logic.

### Text Fields

Available for: `Name()`, `City()`, `Country()`, `State()`, `Category()`, `Source()`, `Email()`, `Phone()`, `Website()`

| Method | Description |
|--------|-------------|
| `Eq(value)` | Equals |
| `Neq(value)` | Not equals |
| `Contains(value)` | Contains substring |
| `NotContains(value)` | Does not contain substring |
| `IsEmpty()` | Field is empty |
| `IsNotEmpty()` | Field is not empty |

```go
// Examples
leadsdb.City().Eq("Berlin")
leadsdb.Name().Contains("Tech")
leadsdb.Email().IsNotEmpty()
```

### Number Fields

Available for: `Rating()`, `ReviewCount()`

| Method | Description |
|--------|-------------|
| `Eq(value)` | Equals |
| `Neq(value)` | Not equals |
| `Gt(value)` | Greater than |
| `Gte(value)` | Greater than or equal |
| `Lt(value)` | Less than |
| `Lte(value)` | Less than or equal |

```go
// Examples
leadsdb.Rating().Gte(4.0)
leadsdb.ReviewCount().Gt(100)
```

### Array Fields

Available for: `Tags()`

| Method | Description |
|--------|-------------|
| `Contains(value)` | Array contains value |
| `NotContains(value)` | Array does not contain value |
| `IsEmpty()` | Array is empty |
| `IsNotEmpty()` | Array is not empty |

```go
// Examples
leadsdb.Tags().Contains("enterprise")
leadsdb.Tags().IsNotEmpty()
```

### Location

| Method | Description |
|--------|-------------|
| `WithinRadius(lat, lon, km)` | Within radius in kilometers |
| `IsSet()` | Coordinates are set |
| `IsNotSet()` | Coordinates are not set |

```go
// Find leads within 50km of Berlin
leadsdb.Location().WithinRadius(52.52, 13.405, 50)
```

### Custom Attributes

Use `Attr(name)` for custom attribute filters:

| Method | Description |
|--------|-------------|
| `Eq(value)` | Text equals |
| `Neq(value)` | Text not equals |
| `Contains(value)` | Text contains |
| `EqNumber(value)` | Number equals |
| `Gt(value)` | Number greater than |
| `Gte(value)` | Number greater than or equal |
| `Lt(value)` | Number less than |
| `Lte(value)` | Number less than or equal |

```go
// Examples
leadsdb.Attr("industry").Eq("Software")
leadsdb.Attr("employees").Gte(100)
```

### OR Logic

```go
// City is Berlin OR Paris
leadsdb.Or().City().Eq("Berlin")
leadsdb.Or().City().Eq("Paris")

// Combined with AND
client.List(ctx,
    leadsdb.Rating().Gte(4.0),         // AND
    leadsdb.Or().City().Eq("Berlin"),  // OR
    leadsdb.Or().City().Eq("Paris"),   // OR
)
```

## Sorting

```go
// Sort by field
leadsdb.Sort(leadsdb.FieldName, leadsdb.Asc)
leadsdb.Sort(leadsdb.FieldRating, leadsdb.Desc)
leadsdb.Sort(leadsdb.FieldCreatedAt, leadsdb.Desc)

// Sort by custom attribute
leadsdb.Sort(leadsdb.AttrSortField("employees"), leadsdb.Desc)
```

Available sort fields:
- `FieldName`, `FieldCity`, `FieldCountry`, `FieldState`
- `FieldCategory`, `FieldSource`, `FieldEmail`, `FieldPhone`, `FieldWebsite`
- `FieldRating`, `FieldReviewCount`
- `FieldCreatedAt`, `FieldUpdatedAt`

## Bulk Operations

### Bulk Create (up to 100 leads)

```go
result, err := client.BulkCreate(ctx, []*leadsdb.Lead{
    {Name: "Lead 1", Source: "import", City: "Athens"},
    {Name: "Lead 2", Source: "import", City: "London"},
    {Name: "Lead 3", Source: "import", City: "Paris"},
})

fmt.Printf("Created: %d, Failed: %d\n", result.Success, result.Failed)
for _, created := range result.Created {
    fmt.Printf("ID: %s (index %d)\n", created.ID, created.Index)
}
```

### Bulk Create from Channel (Streaming)

Auto-batches leads and flushes when batch is full or after timeout:

```go
leads := make(chan *leadsdb.Lead)

go func() {
    defer close(leads)
    for i := range 1000 {
        leads <- &leadsdb.Lead{
            Name:   fmt.Sprintf("Lead %d", i),
            Source: "import",
        }
    }
}()

results, errs := client.BulkCreateFromChan(ctx, leads)

go func() {
    for err := range errs {
        log.Printf("Error: %v", err)
    }
}()

for result := range results {
    fmt.Printf("Created: %s\n", result.ID)
}
```

## Notes

```go
// Create a note
note, err := client.CreateNote(ctx, "lead-id", "Initial contact made")

// List notes
notes, err := client.ListNotes(ctx, "lead-id")

// Update a note
note, err = client.UpdateNote(ctx, "note-id", "Updated content")

// Delete a note
err = client.DeleteNote(ctx, "note-id")
```

## Export

```go
reader, err := client.Export(ctx, leadsdb.ExportCSV)
if err != nil {
    panic(err)
}
defer reader.Close()

// Stream to file
file, _ := os.Create("leads.csv")
io.Copy(file, reader)
```

Export formats: `ExportCSV`, `ExportJSON`, `ExportXLSX`

## Error Handling

```go
lead, err := client.Get(ctx, "non-existent-id")
if errors.Is(err, leadsdb.ErrNotFound) {
    fmt.Println("Lead not found")
}

// API errors include details
var apiErr *leadsdb.APIError
if errors.As(err, &apiErr) {
    fmt.Printf("Status: %d, Message: %s\n", apiErr.StatusCode, apiErr.Message)
}
```

Sentinel errors:
- `ErrNotFound` - Resource not found (404)
- `ErrUnauthorized` - Invalid API key (401)
- `ErrForbidden` - Access denied (403)
- `ErrRateLimited` - Too many requests (429)
- `ErrBadRequest` - Invalid request (400)

## License

MIT
