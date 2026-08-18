// Package equiv is a client for Purdue University's public Transfer Credit
// Course Equivalency Guide.
//
// The guide answers two questions. Forward: "my school's course X — what does
// it become at Purdue?" Reverse: "I need Purdue course Y — who offers something
// that articulates to it?" Both are served by the same Banner form; see Report.
package equiv

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// BaseURL is the Banner application root. Callers may override it in tests.
var BaseURL = "https://selfservice.mypurdue.purdue.edu/prod/"

const (
	ajaxPath   = "bzwtxcrd.p_ajax"
	reportPath = "bzwtxcrd.p_display_report"
	formPath   = "bzwtxcrd.p_select_info"

	// userAgent is sent on every request: the service closes the connection on
	// Go's default agent string.
	userAgent = "purdue-equivalency-cli (+https://github.com/upuddu/purdue-equivalency-cli)"

	maxAttempts = 3
	// formRows is the number of query rows the report form renders per section.
	// Banner reads them positionally and rejects a body with fewer.
	formRows = 5
)

// Option is one entry of a dropdown: a display name and the code the form
// expects for it. School codes in particular are opaque (e.g. "001846"), so the
// value is rarely guessable from the text.
type Option struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

// Equivalency is one articulation: a course at another institution and the
// Purdue credit it converts to.
//
// A single transfer course may produce several Equivalencies. A 4-credit course
// matching a 3-credit Purdue course yields both the named equivalent and an
// "XTRA" row carrying the leftover credit hour; Continuation marks the latter.
type Equivalency struct {
	TransferSchool  string `json:"transfer_school"`
	TransferSubject string `json:"transfer_subject"`
	TransferCourse  string `json:"transfer_course"`
	TransferTitle   string `json:"transfer_title"`
	TransferCredits string `json:"transfer_credits"`
	PurdueSubject   string `json:"purdue_subject"`
	PurdueCourse    string `json:"purdue_course"`
	PurdueTitle     string `json:"purdue_title"`
	PurdueCredits   string `json:"purdue_credits"`

	Continuation bool   `json:"continuation,omitempty"`
	Location     string `json:"location,omitempty"`
	State        string `json:"state,omitempty"`
}

// Client talks to the equivalency guide. The zero value is not usable; call New.
// A Client is safe for concurrent use.
type Client struct {
	httpClient *http.Client

	mu       sync.Mutex
	options  map[string][]Option // keyed by request URL
	subjects []Option
}

// New returns a Client using an HTTP client with a request timeout.
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 45 * time.Second},
		options:    make(map[string][]Option),
	}
}

// Locations returns the two top-level regions the guide partitions schools by.
// It is a constant in the page markup rather than a served list.
func (c *Client) Locations() []Option {
	return []Option{{Text: "US", Value: "US"}, {Text: "Outside US", Value: "Outside US"}}
}

// States lists states (or countries, outside the US) that have any school on file.
func (c *Client) States(ctx context.Context, location string) ([]Option, error) {
	return c.dropdown(ctx, "states", location, "", "", "")
}

// Schools lists institutions in a state, keyed by the code Report expects.
func (c *Client) Schools(ctx context.Context, state, location string) ([]Option, error) {
	return c.dropdown(ctx, "school", state, location, "", "")
}

// SchoolSubjects lists the subject prefixes a school has articulations for.
func (c *Client) SchoolSubjects(ctx context.Context, schoolCode string) ([]Option, error) {
	return c.dropdown(ctx, "subject", schoolCode, "", "", "")
}

// SchoolCourses lists a school's courses within one subject.
func (c *Client) SchoolCourses(ctx context.Context, subject, schoolCode string) ([]Option, error) {
	return c.dropdown(ctx, "course", subject, schoolCode, "", "")
}

// PurdueSubjects lists Purdue subject prefixes that have at least one
// articulation. Unlike every other dropdown this one is rendered into the search
// form server-side, so it has to be scraped rather than requested.
func (c *Client) PurdueSubjects(ctx context.Context) ([]Option, error) {
	c.mu.Lock()
	cached := c.subjects
	c.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	body, err := c.get(ctx, BaseURL+formPath)
	if err != nil {
		return nil, fmt.Errorf("fetch search form: %w", err)
	}
	subjects, err := parseSubjectOptions(body)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.subjects = subjects
	c.mu.Unlock()
	return subjects, nil
}

// PurdueCourses lists course numbers within a Purdue subject that have at least
// one articulation on file.
func (c *Client) PurdueCourses(ctx context.Context, subject string) ([]Option, error) {
	return c.dropdown(ctx, "purdue_course", subject, "", "", "")
}

// PurdueLocations lists the regions holding an articulation for a Purdue course.
func (c *Client) PurdueLocations(ctx context.Context, subject, course string) ([]Option, error) {
	return c.dropdown(ctx, "location", course, subject, "", "")
}

// PurdueStates lists the states within a region holding an articulation.
func (c *Client) PurdueStates(ctx context.Context, subject, course, location string) ([]Option, error) {
	return c.dropdown(ctx, "state", location, subject, course, "")
}

// PurdueSchools lists the schools within a state holding an articulation.
func (c *Client) PurdueSchools(ctx context.Context, subject, course, location, state string) ([]Option, error) {
	return c.dropdown(ctx, "purdue_schools", state, subject, course, location)
}

// ForwardQuery selects a course at another institution. School is the opaque
// code from Schools. Course accepts "ALL" for every course in the subject.
type ForwardQuery struct {
	Location string
	State    string
	School   string
	Subject  string
	Course   string
}

// ReverseQuery selects a Purdue course. Course and School each accept "ALL".
type ReverseQuery struct {
	Subject  string
	Course   string
	Location string
	State    string
	School   string
}

// Report runs the equivalency search. Either or both query sets may be empty,
// and each is capped at formRows entries; extras are ignored.
func (c *Client) Report(ctx context.Context, forward []ForwardQuery, reverse []ReverseQuery) ([]Equivalency, error) {
	body, err := c.post(ctx, BaseURL+reportPath, encodeReportForm(forward, reverse))
	if err != nil {
		return nil, err
	}
	return parseReport(body), nil
}

// encodeReportForm builds the POST body. Banner identifies a field by its
// position among same-named fields, so all formRows rows of both sections are
// emitted in form order — a body containing only the populated row is rejected.
func encodeReportForm(forward []ForwardQuery, reverse []ReverseQuery) string {
	var b strings.Builder
	field := func(name, value string) {
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(value))
	}

	for i := 0; i < formRows; i++ {
		var q ForwardQuery
		if i < len(forward) {
			q = forward[i]
		}
		field("location_in", q.Location)
		field("state_in", q.State)
		field("school_in", q.School)
		field("subject_in", q.Subject)
		field("course_in", q.Course)
	}
	for i := 0; i < formRows; i++ {
		var q ReverseQuery
		if i < len(reverse) {
			q = reverse[i]
		}
		field("purdue_subject_in", q.Subject)
		field("purdue_course_in", q.Course)
		field("purdue_location_in", q.Location)
		field("purdue_state_in", q.State)
		field("purdue_school_in", q.School)
	}
	return b.String()
}

// WhoOffers finds every articulation that yields the given Purdue course.
//
// The guide has no "search everywhere" mode, so this walks the location and
// state dropdowns — which only list places that actually hold an articulation —
// and runs one report per state. Pass a non-empty state (name or code) to narrow
// the sweep to a single one.
func (c *Client) WhoOffers(ctx context.Context, subject, course, state string) ([]Equivalency, error) {
	locations, err := c.PurdueLocations(ctx, subject, course)
	if err != nil {
		return nil, err
	}

	type region struct{ location, state string }
	var regions []region
	for _, loc := range locations {
		states, err := c.PurdueStates(ctx, subject, course, loc.Value)
		if err != nil {
			return nil, err
		}
		for _, st := range states {
			if state != "" && !strings.EqualFold(st.Value, state) && !strings.EqualFold(st.Text, state) {
				continue
			}
			regions = append(regions, region{loc.Value, st.Value})
		}
	}

	results := make([][]Equivalency, len(regions))
	errs := make([]error, len(regions))

	// Bounded concurrency: enough to hide latency, gentle on a public service.
	const maxInFlight = 4
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup

	for i, r := range regions {
		wg.Add(1)
		go func(i int, r region) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			rows, err := c.Report(ctx, nil, []ReverseQuery{{
				Subject: subject, Course: course,
				Location: r.location, State: r.state, School: "ALL",
			}})
			if err != nil {
				errs[i] = fmt.Errorf("%s/%s: %w", r.location, r.state, err)
				return
			}
			for j := range rows {
				rows[j].Location = r.location
				rows[j].State = r.state
			}
			results[i] = rows
		}(i, r)
	}
	wg.Wait()

	var all []Equivalency
	for _, rows := range results {
		all = append(all, rows...)
	}
	// Partial results still answer the question, so only surface a failure when
	// the sweep turned up nothing at all.
	if len(all) == 0 {
		for _, err := range errs {
			if err != nil {
				return nil, err
			}
		}
	}
	return all, nil
}

// dropdown fetches one dependent dropdown. The endpoint takes four positional
// values whose meaning depends on requestType, and answers with the id of the
// element to populate followed by "text~value" lines.
func (c *Client) dropdown(ctx context.Context, requestType, v1, v2, v3, v4 string) ([]Option, error) {
	query := url.Values{
		"request_type":   {requestType},
		"request_value":  {v1},
		"request_value2": {v2},
		"request_value3": {v3},
		"request_value4": {v4},
		"load_into":      {"target"},
	}
	endpoint := BaseURL + ajaxPath + "?" + query.Encode()

	c.mu.Lock()
	cached, ok := c.options[endpoint]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}

	body, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("%s dropdown: %w", requestType, err)
	}
	opts := parseOptions(body)

	c.mu.Lock()
	c.options[endpoint] = opts
	c.mu.Unlock()
	return opts, nil
}

// parseOptions reads the "text~value" list, skipping the leading element id.
func parseOptions(body string) []Option {
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		return nil
	}
	var opts []Option
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(strings.TrimSpace(line), "~")
		if !ok || value == "" {
			continue
		}
		opts = append(opts, Option{
			Text:  html.UnescapeString(name),
			Value: html.UnescapeString(value),
		})
	}
	return opts
}

func (c *Client) get(ctx context.Context, endpoint string) (string, error) {
	return c.do(ctx, http.MethodGet, endpoint, "")
}

func (c *Client) post(ctx context.Context, endpoint, body string) (string, error) {
	return c.do(ctx, http.MethodPost, endpoint, body)
}

// do issues a request, retrying transport errors and 5xx responses. The service
// is a legacy Banner instance that fails intermittently under light load.
func (c *Client) do(ctx context.Context, method, endpoint, body string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 750 * time.Millisecond
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", userAgent)
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		payload, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return string(payload), nil
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		default:
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
	}
	return "", lastErr
}
