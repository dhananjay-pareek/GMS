package leadsdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// DefaultBaseURL is the default API base URL.
	DefaultBaseURL = "https://getleadsdb.com/api/v1"
	// DefaultTimeout is the default HTTP client timeout.
	DefaultTimeout = 60 * time.Second
	// DefaultMaxRetries is the default number of retry attempts.
	DefaultMaxRetries = 3
	// DefaultBaseDelay is the base delay for exponential backoff.
	DefaultBaseDelay = 1 * time.Second
	// DefaultFlushTimeout is the default timeout for flushing partial batches.
	DefaultFlushTimeout = 2 * time.Second
	// maxJitter is the maximum jitter added to backoff.
	maxJitter = 500 * time.Millisecond
	// maxBatchSize is the maximum number of leads per batch.
	maxBatchSize = 100
)

// Client is the LeadsDB API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	maxRetries int
}

// Option configures the Client.
type Option func(*Client)

// New creates a new LeadsDB client with the given API key and options.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		apiKey:     apiKey,
		maxRetries: DefaultMaxRetries,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithBaseURL sets a custom base URL for the API.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		c.maxRetries = n
	}
}

// Get retrieves a lead by ID.
func (c *Client) Get(ctx context.Context, id string) (*Lead, error) {
	if id == "" {
		return nil, errors.New("leadsdb: id is required")
	}

	var lead Lead
	if err := c.do(ctx, http.MethodGet, "/leads/"+id, nil, &lead); err != nil {
		return nil, err
	}

	return &lead, nil
}

// Update partially updates a lead by ID.
func (c *Client) Update(ctx context.Context, id string, input *UpdateLeadInput) (*Lead, error) {
	if id == "" {
		return nil, errors.New("leadsdb: id is required")
	}
	if input == nil {
		return nil, errors.New("leadsdb: input is required")
	}

	var lead Lead
	if err := c.do(ctx, http.MethodPatch, "/leads/"+id, input, &lead); err != nil {
		return nil, err
	}

	return &lead, nil
}

// Create creates a new lead.
func (c *Client) Create(ctx context.Context, lead *Lead) (*Lead, error) {
	if lead == nil {
		return nil, errors.New("leadsdb: lead is required")
	}
	if lead.Name == "" {
		return nil, errors.New("leadsdb: name is required")
	}
	if lead.Source == "" {
		return nil, errors.New("leadsdb: source is required")
	}

	var created Lead
	if err := c.do(ctx, http.MethodPost, "/leads", lead, &created); err != nil {
		return nil, err
	}

	return &created, nil
}

// Delete deletes a lead by ID.
func (c *Client) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("leadsdb: id is required")
	}

	return c.do(ctx, http.MethodDelete, "/leads/"+id, nil, nil)
}

// CreateNote creates a note for a lead.
func (c *Client) CreateNote(ctx context.Context, leadID, content string) (*Note, error) {
	if leadID == "" {
		return nil, errors.New("leadsdb: leadID is required")
	}
	if content == "" {
		return nil, errors.New("leadsdb: content is required")
	}

	var note Note
	if err := c.do(ctx, http.MethodPost, "/leads/"+leadID+"/notes", createNoteRequest{Content: content}, &note); err != nil {
		return nil, err
	}

	return &note, nil
}

// ListNotes returns all notes for a lead.
func (c *Client) ListNotes(ctx context.Context, leadID string) ([]Note, error) {
	if leadID == "" {
		return nil, errors.New("leadsdb: leadID is required")
	}

	var notes []Note
	if err := c.do(ctx, http.MethodGet, "/leads/"+leadID+"/notes", nil, &notes); err != nil {
		return nil, err
	}

	return notes, nil
}

// UpdateNote updates a note's content.
func (c *Client) UpdateNote(ctx context.Context, noteID, content string) (*Note, error) {
	if noteID == "" {
		return nil, errors.New("leadsdb: noteID is required")
	}
	if content == "" {
		return nil, errors.New("leadsdb: content is required")
	}

	var note Note
	if err := c.do(ctx, http.MethodPut, "/leads/notes/"+noteID, createNoteRequest{Content: content}, &note); err != nil {
		return nil, err
	}

	return &note, nil
}

// DeleteNote deletes a note.
func (c *Client) DeleteNote(ctx context.Context, noteID string) error {
	if noteID == "" {
		return errors.New("leadsdb: noteID is required")
	}

	return c.do(ctx, http.MethodDelete, "/leads/notes/"+noteID, nil, nil)
}

// ExportFormat defines the format for exporting leads.
type ExportFormat string

const (
	ExportCSV  ExportFormat = "csv"
	ExportJSON ExportFormat = "json"
)

// Export exports leads in the specified format and returns a reader.
// The caller must close the reader when done.
func (c *Client) Export(ctx context.Context, format ExportFormat) (io.ReadCloser, error) {
	if format == "" {
		format = ExportCSV
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/leads/export?format="+string(format), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return resp.Body, nil
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	apiErr := &APIError{StatusCode: resp.StatusCode}
	if len(body) > 0 {
		_ = json.Unmarshal(body, apiErr)
	}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(resp.StatusCode)
	}

	return nil, apiErr
}

// BulkCreate creates up to 100 leads in a single request.
func (c *Client) BulkCreate(ctx context.Context, leads []*Lead) (*BulkCreateResult, error) {
	if len(leads) == 0 {
		return nil, errors.New("leadsdb: leads is required")
	}
	if len(leads) > maxBatchSize {
		return nil, errors.New("leadsdb: maximum 100 leads allowed")
	}
	for i, lead := range leads {
		if lead.Name == "" {
			return nil, fmt.Errorf("leadsdb: lead at index %d: name is required", i)
		}
		if lead.Source == "" {
			return nil, fmt.Errorf("leadsdb: lead at index %d: source is required", i)
		}
	}

	body := struct {
		Leads []*Lead `json:"leads"`
	}{Leads: leads}

	var result BulkCreateResult
	if err := c.do(ctx, http.MethodPost, "/leads/batch", body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SortOrder defines the order for sorting.
type SortOrder string

const (
	Asc  SortOrder = "ASC"
	Desc SortOrder = "DESC"
)

// ListResult contains the result of a list operation.
type ListResult struct {
	Leads      []Lead `json:"leads"`
	Count      int    `json:"count"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor"`
}

// ListOption configures the List method.
type ListOption interface {
	apply(*listConfig)
}

type listConfig struct {
	limit     int
	cursor    string
	sortBy    string
	sortOrder SortOrder
	filters   []filter
}

type limitOption int

func (o limitOption) apply(cfg *listConfig) { cfg.limit = int(o) }

// Limit sets the maximum number of results.
func Limit(n int) ListOption { return limitOption(n) }

type cursorOption string

func (o cursorOption) apply(cfg *listConfig) { cfg.cursor = string(o) }

// Cursor sets the pagination cursor.
func Cursor(c string) ListOption { return cursorOption(c) }

type sortOption struct {
	field string
	order SortOrder
}

func (o sortOption) apply(cfg *listConfig) {
	cfg.sortBy = o.field
	cfg.sortOrder = o.order
}

// Sort sets the sort field and order.
func Sort(field SortField, order SortOrder) ListOption {
	return sortOption{field: field.sortFieldName(), order: order}
}

// List retrieves leads with optional filtering, sorting, and pagination.
func (c *Client) List(ctx context.Context, opts ...ListOption) (*ListResult, error) {
	cfg := &listConfig{}
	for _, opt := range opts {
		opt.apply(cfg)
	}

	params := url.Values{}

	if cfg.limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", cfg.limit))
	}
	if cfg.cursor != "" {
		params.Set("cursor", cfg.cursor)
	}
	if cfg.sortBy != "" {
		params.Set("sort_by", cfg.sortBy)
		if cfg.sortOrder != "" {
			params.Set("sort_order", string(cfg.sortOrder))
		}
	}
	for _, f := range cfg.filters {
		params.Add("filter", f.String())
	}

	path := "/leads"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var result ListResult
	if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Iterator returns an iterator that yields leads matching the options.
// It handles pagination automatically.
func (c *Client) Iterator(ctx context.Context, opts ...ListOption) iter.Seq2[*Lead, error] {
	return func(yield func(*Lead, error) bool) {
		for lead, err := range c.iterate(ctx, opts) {
			if !yield(lead, err) {
				return
			}
			if err != nil {
				return
			}
		}
	}
}

// IteratorChan returns channels that yield leads matching the options.
// It handles pagination automatically in a goroutine.
// Both channels are closed when all leads are processed or the context is cancelled.
func (c *Client) IteratorChan(ctx context.Context, opts ...ListOption) (<-chan *Lead, <-chan error) {
	leads := make(chan *Lead)
	errs := make(chan error, 1)

	go func() {
		defer close(leads)
		defer close(errs)

		for lead, err := range c.iterate(ctx, opts) {
			if err != nil {
				select {
				case errs <- err:
				case <-ctx.Done():
				}
				return
			}

			select {
			case leads <- lead:
			case <-ctx.Done():
				return
			}
		}
	}()

	return leads, errs
}

func (c *Client) iterate(ctx context.Context, opts []ListOption) iter.Seq2[*Lead, error] {
	return func(yield func(*Lead, error) bool) {
		cursor := ""
		for {
			listOpts := make([]ListOption, len(opts), len(opts)+1)
			copy(listOpts, opts)
			if cursor != "" {
				listOpts = append(listOpts, Cursor(cursor))
			}

			result, err := c.List(ctx, listOpts...)
			if err != nil {
				yield(nil, err)
				return
			}

			for i := range result.Leads {
				if !yield(&result.Leads[i], nil) {
					return
				}
			}

			if !result.HasMore {
				return
			}
			cursor = result.NextCursor
		}
	}
}

// BulkCreateChanOption configures the BulkCreateFromChan method.
type BulkCreateChanOption func(*bulkCreateChanConfig)

type bulkCreateChanConfig struct {
	flushTimeout time.Duration
}

// WithFlushTimeout sets the timeout for flushing partial batches.
func WithFlushTimeout(d time.Duration) BulkCreateChanOption {
	return func(cfg *bulkCreateChanConfig) {
		cfg.flushTimeout = d
	}
}

// BulkCreateFromChan reads leads from the input channel and creates them in batches of 100.
// It returns a channel of results for each successfully created lead and a channel for errors.
// Both channels are closed when all leads are processed or the context is cancelled.
func (c *Client) BulkCreateFromChan(ctx context.Context, leads <-chan *Lead, opts ...BulkCreateChanOption) (<-chan *BulkLeadResult, <-chan error) {
	cfg := &bulkCreateChanConfig{
		flushTimeout: DefaultFlushTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	results := make(chan *BulkLeadResult)
	errs := make(chan error, 1)

	go func() {
		defer close(results)
		defer close(errs)

		batch := make([]*Lead, 0, maxBatchSize)
		timer := time.NewTimer(cfg.flushTimeout)
		timer.Stop()
		defer timer.Stop()

		flush := func() {
			if len(batch) == 0 {
				return
			}

			result, err := c.BulkCreate(ctx, batch)
			if err != nil {
				select {
				case errs <- err:
				case <-ctx.Done():
				}
				batch = batch[:0]
				return
			}

			for i := range result.Created {
				select {
				case results <- &result.Created[i]:
				case <-ctx.Done():
					return
				}
			}

			for i := range result.Errors {
				select {
				case errs <- fmt.Errorf("index %d: %s", result.Errors[i].Index, result.Errors[i].Message):
				case <-ctx.Done():
					return
				}
			}

			batch = batch[:0]
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				flush()
			case lead, ok := <-leads:
				if !ok {
					timer.Stop()
					flush()
					return
				}

				batch = append(batch, lead)
				if len(batch) == 1 {
					timer.Reset(cfg.flushTimeout)
				}
				if len(batch) >= maxBatchSize {
					timer.Stop()
					flush()
				}
			}
		}
	}()

	return results, errs
}

func (c *Client) do(ctx context.Context, method, path string, body, result any) error {
	var bodyData []byte
	if body != nil {
		var err error
		bodyData, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	var lastErr error
	for attempt := range c.maxRetries {
		if err := ctx.Err(); err != nil {
			return err
		}

		var bodyReader io.Reader
		if bodyData != nil {
			bodyReader = bytes.NewReader(bodyData)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return err
		}

		req.Header.Set("X-API-Key", c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if !c.shouldRetry(0, err) {
				return err
			}
			c.backoff(ctx, attempt, 0)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			if result != nil && len(respBody) > 0 {
				return json.Unmarshal(respBody, result)
			}
			return nil
		}

		apiErr := &APIError{StatusCode: resp.StatusCode}
		if len(respBody) > 0 {
			_ = json.Unmarshal(respBody, apiErr)
		}
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(resp.StatusCode)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if seconds, err := strconv.Atoi(ra); err == nil {
					apiErr.RetryAfter = seconds
				}
			}
		}

		lastErr = apiErr

		if !c.shouldRetry(resp.StatusCode, nil) {
			return apiErr
		}

		c.backoff(ctx, attempt, apiErr.RetryAfter)
	}

	return lastErr
}

func (c *Client) shouldRetry(statusCode int, err error) bool {
	if err != nil {
		return true
	}

	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (c *Client) backoff(ctx context.Context, attempt, retryAfter int) {
	var delay time.Duration

	if retryAfter > 0 {
		delay = time.Duration(retryAfter) * time.Second
	} else {
		delay = DefaultBaseDelay << attempt
	}

	jitter := time.Duration(rand.Int64N(int64(maxJitter)))
	delay += jitter

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
