package leadsdb

// Note represents a note attached to a lead.
type Note struct {
	ID        string   `json:"id"`
	LeadID    string   `json:"lead_id"`
	Content   string   `json:"content"`
	CreatedAt UnixTime `json:"created_at"`
	UpdatedAt UnixTime `json:"updated_at"`
}

type createNoteRequest struct {
	Content string `json:"content"`
}
