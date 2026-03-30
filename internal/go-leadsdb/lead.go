// Package leadsdb defines the data structures for managing business leads.
package leadsdb

// Lead represents a business lead in the system.
type Lead struct {
	// Core identifiers
	// ID is the unique identifier for the lead.
	ID string `json:"id"`
	// Name is the name of the business.
	Name string `json:"name"`
	// Source indicates where the lead was sourced from.
	Source string `json:"source"`

	// Optional description
	// Description provides additional details about the lead.
	Description string `json:"description,omitempty"`

	// Location fields
	// Address is the street address of the business.
	Address string `json:"address,omitempty"`
	// City is the city where the business is located.
	City string `json:"city,omitempty"`
	// State is the state or province of the business location.
	State string `json:"state,omitempty"`
	// Country is the country of the business location.
	Country string `json:"country,omitempty"`
	// PostalCode is the postal or ZIP code of the business location.
	PostalCode string `json:"postal_code,omitempty"`
	// Latitude is the latitude of the business location.
	Latitude *float64 `json:"latitude,omitempty"`
	// Longitude is the longitude of the business location.
	Longitude *float64 `json:"longitude,omitempty"`

	// Contact information
	// Phone is the contact phone number for the business.
	Phone string `json:"phone,omitempty"`
	// Email is the contact email address for the business.
	Email string `json:"email,omitempty"`
	// Website is the business's website URL.
	Website string `json:"website,omitempty"`

	// Business metrics
	// Rating is the average customer rating for the business.
	Rating *float64 `json:"rating,omitempty"`
	// ReviewCount is the number of customer reviews for the business.
	ReviewCount *int `json:"review_count,omitempty"`

	// Categorization
	// Category is the primary category of the business.
	Category string `json:"category,omitempty"`
	// Tags are additional tags associated with the business.
	Tags []string `json:"tags,omitempty"`

	// Source tracking
	// SourceID is the identifier from the source system.
	SourceID string `json:"source_id,omitempty"`
	// LogoURL is the URL to the business's logo image.
	LogoURL string `json:"logo_url,omitempty"`

	// Dynamic attributes
	// Attributes are custom attributes associated with the lead.
	Attributes []Attribute `json:"attributes,omitempty"`

	// Associated notes
	// Notes are notes linked to the lead.
	Notes []Note `json:"notes,omitempty"`

	// Timestamps
	// CreatedAt is the timestamp when the lead was created.
	CreatedAt UnixTime `json:"created_at"`
	// UpdatedAt is the timestamp when the lead was last updated.
	UpdatedAt UnixTime `json:"updated_at"`
}

// UpdateLeadInput contains the fields for updating an existing lead.
// All fields are optional; only non-nil fields will be updated.
type UpdateLeadInput struct {
	Name        *string `json:"name,omitempty"`
	Source      *string `json:"source,omitempty"`
	Description *string `json:"description,omitempty"`

	// Location fields
	Address    *string  `json:"address,omitempty"`
	City       *string  `json:"city,omitempty"`
	State      *string  `json:"state,omitempty"`
	Country    *string  `json:"country,omitempty"`
	PostalCode *string  `json:"postal_code,omitempty"`
	Latitude   *float64 `json:"latitude,omitempty"`
	Longitude  *float64 `json:"longitude,omitempty"`

	// Contact information
	Phone   *string `json:"phone,omitempty"`
	Email   *string `json:"email,omitempty"`
	Website *string `json:"website,omitempty"`

	// Business metrics
	Rating      *float64 `json:"rating,omitempty"`
	ReviewCount *int     `json:"review_count,omitempty"`

	// Categorization
	Category *string  `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`

	// Source tracking
	SourceID *string `json:"source_id,omitempty"`
	LogoURL  *string `json:"logo_url,omitempty"`

	// Dynamic attributes (replaces all existing attributes)
	Attributes []Attribute `json:"attributes,omitempty"`
}

// BulkCreateResult contains the result of a bulk create operation.
type BulkCreateResult struct {
	Total   int              `json:"total"`
	Success int              `json:"success"`
	Failed  int              `json:"failed"`
	Created []BulkLeadResult `json:"created"`
	Errors  []BulkLeadError  `json:"errors"`
}

// BulkLeadResult contains the result of a successfully created lead.
type BulkLeadResult struct {
	Index     int      `json:"index"`
	ID        string   `json:"id"`
	CreatedAt UnixTime `json:"created_at"`
}

// BulkLeadError contains the error for a failed lead creation.
type BulkLeadError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}
