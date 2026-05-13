package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL        = "https://export.arxiv.org/api/query"
	defaultUserAgent      = "fieldwork/1.0 (https://github.com/meistro57/fieldwork)"
	defaultRequestSpacing = 3 * time.Second
	max429Retries         = 3
	initial429Backoff     = 60 * time.Second
)

type Paper struct {
	ID          string
	Title       string
	Authors     []string
	Abstract    string
	Categories  []string
	SubmittedAt time.Time
	PDFURL      string
}

type Client struct {
	BaseURL         string
	HTTPClient      *http.Client
	Categories      []string
	UserAgent       string
	RequestInterval time.Duration

	lastRequest time.Time
}

func NewClient(categories []string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		BaseURL:         defaultBaseURL,
		HTTPClient:      httpClient,
		Categories:      categories,
		UserAgent:       defaultUserAgent,
		RequestInterval: defaultRequestSpacing,
	}
}

func (c *Client) FetchLatest(ctx context.Context, categories []string, limit int) ([]Paper, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}

	selectedCategories := categories
	if len(selectedCategories) == 0 {
		selectedCategories = c.Categories
	}
	if len(selectedCategories) == 0 {
		return nil, fmt.Errorf("at least one arXiv category is required")
	}

	// Query each category separately and merge — multi-category OR queries
	// trigger 429/503 on arXiv's API even when single-category queries succeed.
	perCat := (limit + len(selectedCategories) - 1) / len(selectedCategories)
	seen := make(map[string]bool)
	all := make([]Paper, 0, limit)

	for _, category := range selectedCategories {
		clean := strings.TrimSpace(category)
		if clean == "" {
			continue
		}
		papers, err := c.fetchCategory(ctx, clean, perCat)
		if err != nil {
			return nil, fmt.Errorf("fetch category %s: %w", clean, err)
		}
		for _, p := range papers {
			if !seen[p.ID] {
				seen[p.ID] = true
				all = append(all, p)
			}
		}
		if len(all) >= limit {
			break
		}
	}

	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (c *Client) fetchCategory(ctx context.Context, category string, limit int) ([]Paper, error) {
	params := url.Values{}
	params.Set("search_query", "cat:"+category)
	params.Set("sortBy", "submittedDate")
	params.Set("sortOrder", "descending")
	params.Set("start", "0")
	params.Set("max_results", fmt.Sprintf("%d", limit))

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
			return nil, fmt.Errorf("request arXiv API: %w", err)
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
			return nil, fmt.Errorf("read arXiv response: %w", err)
		}

		var feed atomFeed
		if err := xml.Unmarshal(data, &feed); err != nil {
			return nil, fmt.Errorf("parse arXiv atom feed: %w", err)
		}

		papers := make([]Paper, 0, len(feed.Entries))
		for _, entry := range feed.Entries {
			submitted, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.Published))
			if err != nil {
				return nil, fmt.Errorf("parse submitted timestamp for %q: %w", entry.ID, err)
			}

			paper := Paper{
				ID:          extractArxivID(entry.ID),
				Title:       collapseWhitespace(entry.Title),
				Authors:     extractAuthors(entry.Authors),
				Abstract:    collapseWhitespace(entry.Summary),
				Categories:  extractCategories(entry.Categories),
				SubmittedAt: submitted,
				PDFURL:      extractPDFURL(entry),
			}

			if paper.PDFURL == "" {
				paper.PDFURL = toPDFURL(entry.ID)
			}

			papers = append(papers, paper)
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
	return fmt.Errorf("arXiv API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID         string         `xml:"id"`
	Title      string         `xml:"title"`
	Summary    string         `xml:"summary"`
	Published  string         `xml:"published"`
	Authors    []atomAuthor   `xml:"author"`
	Categories []atomCategory `xml:"category"`
	Links      []atomLink     `xml:"link"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomCategory struct {
	Term string `xml:"term,attr"`
}

type atomLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
}

func extractAuthors(input []atomAuthor) []string {
	out := make([]string, 0, len(input))
	for _, author := range input {
		name := strings.TrimSpace(author.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func extractCategories(input []atomCategory) []string {
	out := make([]string, 0, len(input))
	for _, category := range input {
		term := strings.TrimSpace(category.Term)
		if term != "" {
			out = append(out, term)
		}
	}
	return out
}

func extractPDFURL(entry atomEntry) string {
	for _, link := range entry.Links {
		href := strings.TrimSpace(link.Href)
		if href == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(link.Title), "pdf") {
			return href
		}
		if strings.EqualFold(strings.TrimSpace(link.Type), "application/pdf") {
			return href
		}
	}

	return ""
}

func extractArxivID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err == nil {
		id := path.Base(parsed.Path)
		if id != "" && id != "/" {
			return id
		}
	}

	if i := strings.LastIndex(trimmed, "/"); i >= 0 && i+1 < len(trimmed) {
		return trimmed[i+1:]
	}
	return trimmed
}

func toPDFURL(rawID string) string {
	id := extractArxivID(rawID)
	if id == "" {
		return ""
	}
	return "https://arxiv.org/pdf/" + id + ".pdf"
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
