package semanticscholar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fieldwork/internal/arxiv"
)

const (
	defaultBaseURL        = "https://api.semanticscholar.org/graph/v1/paper/search"
	defaultUserAgent      = "fieldwork/1.0 (https://github.com/meistro57/fieldwork)"
	defaultRequestSpacing = 1 * time.Second
	defaultQuery          = "quantum physics OR quantum mechanics OR condensed matter OR high energy physics"
	defaultFields         = "title,abstract,authors,year,externalIds,openAccessPdf,publicationDate,fieldsOfStudy"
	max429Retries         = 3
	initial429Backoff     = 60 * time.Second
)

type Client struct {
	BaseURL         string
	HTTPClient      *http.Client
	UserAgent       string
	RequestInterval time.Duration
	DefaultQuery    string

	lastRequest time.Time
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		BaseURL:         defaultBaseURL,
		HTTPClient:      httpClient,
		UserAgent:       defaultUserAgent,
		RequestInterval: defaultRequestSpacing,
		DefaultQuery:    defaultQuery,
	}
}

func (c *Client) FetchLatest(ctx context.Context, categories []string, limit int) ([]arxiv.Paper, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}

	query := strings.TrimSpace(c.DefaultQuery)
	if query == "" {
		query = defaultQuery
	}
	if custom := joinNonEmpty(categories); custom != "" {
		query = custom
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", "0")
	params.Set("fields", defaultFields)
	params.Set("sort", "publicationDate:desc")

	endpoint := strings.TrimRight(c.BaseURL, "?") + "?" + params.Encode()

	for attempt := 0; ; attempt++ {
		if err := c.waitForRateLimit(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		if strings.TrimSpace(c.UserAgent) != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request Semantic Scholar API: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt >= max429Retries {
				errResp := buildStatusError(resp)
				resp.Body.Close()
				return nil, errResp
			}

			waitDuration := parseRetryAfter(resp.Header.Get("Retry-After"))
			if waitDuration <= 0 {
				waitDuration = initial429Backoff << attempt
			}
			resp.Body.Close()

			if err := waitWithContext(ctx, waitDuration); err != nil {
				return nil, err
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errResp := buildStatusError(resp)
			resp.Body.Close()
			return nil, errResp
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read Semantic Scholar response: %w", err)
		}

		var parsed searchResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("parse Semantic Scholar JSON response: %w", err)
		}

		papers := make([]arxiv.Paper, 0, len(parsed.Data))
		for _, item := range parsed.Data {
			id := normalizeArxivID(item.ExternalIDs.ArXiv)
			if id == "" {
				continue
			}

			submittedAt := parsePublicationDate(item.PublicationDate, item.Year)
			pdfURL := strings.TrimSpace(item.OpenAccessPDF.URL)
			if pdfURL == "" {
				pdfURL = toArxivPDFURL(id)
			}

			papers = append(papers, arxiv.Paper{
				ID:          id,
				Title:       collapseWhitespace(item.Title),
				Authors:     extractAuthors(item.Authors),
				Abstract:    collapseWhitespace(item.Abstract),
				Categories:  cleanList(item.FieldsOfStudy),
				SubmittedAt: submittedAt,
				PDFURL:      pdfURL,
			})
		}

		if len(papers) > limit {
			papers = papers[:limit]
		}

		return papers, nil
	}
}

func (c *Client) waitForRateLimit(ctx context.Context) error {
	if c.RequestInterval <= 0 {
		c.RequestInterval = defaultRequestSpacing
	}

	wait := c.RequestInterval
	if !c.lastRequest.IsZero() {
		wait = c.RequestInterval - time.Since(c.lastRequest)
	}

	if wait > 0 {
		if err := waitWithContext(ctx, wait); err != nil {
			return err
		}
	}

	c.lastRequest = time.Now()
	return nil
}

func waitWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseRetryAfter(value string) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}

	seconds, err := strconv.Atoi(trimmed)
	if err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	retryAt, err := http.ParseTime(trimmed)
	if err != nil {
		return 0
	}

	duration := time.Until(retryAt)
	if duration <= 0 {
		return 0
	}
	return duration
}

func buildStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("Semantic Scholar API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

type searchResponse struct {
	Data []paperResult `json:"data"`
}

type paperResult struct {
	Title           string   `json:"title"`
	Abstract        string   `json:"abstract"`
	Authors         []author `json:"authors"`
	Year            int      `json:"year"`
	ExternalIDs     extIDs   `json:"externalIds"`
	OpenAccessPDF   pdfLink  `json:"openAccessPdf"`
	PublicationDate string   `json:"publicationDate"`
	FieldsOfStudy   []string `json:"fieldsOfStudy"`
}

type author struct {
	Name string `json:"name"`
}

type extIDs struct {
	ArXiv string `json:"ArXiv"`
}

type pdfLink struct {
	URL string `json:"url"`
}

func extractAuthors(input []author) []string {
	out := make([]string, 0, len(input))
	for _, item := range input {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func cleanList(input []string) []string {
	out := make([]string, 0, len(input))
	for _, item := range input {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func joinNonEmpty(input []string) string {
	clean := cleanList(input)
	if len(clean) == 0 {
		return ""
	}
	return strings.Join(clean, " OR ")
}

func normalizeArxivID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" {
		return ""
	}
	id = strings.TrimPrefix(id, "arXiv:")
	id = strings.TrimPrefix(id, "https://arxiv.org/abs/")
	id = strings.TrimPrefix(id, "http://arxiv.org/abs/")
	return strings.TrimSpace(id)
}

func toArxivPDFURL(id string) string {
	clean := strings.TrimSpace(id)
	if clean == "" {
		return ""
	}
	return "https://arxiv.org/pdf/" + clean
}

func parsePublicationDate(raw string, year int) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		formats := []string{time.RFC3339, "2006-01-02", "2006-01"}
		for _, format := range formats {
			if parsed, err := time.Parse(format, trimmed); err == nil {
				return parsed
			}
		}
	}

	if year > 0 {
		return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	}

	return time.Time{}
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
