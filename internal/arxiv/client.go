package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const defaultBaseURL = "http://export.arxiv.org/api/query"

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
	BaseURL    string
	HTTPClient *http.Client
	Categories []string
}

func NewClient(categories []string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: httpClient,
		Categories: categories,
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

	query := make([]string, 0, len(selectedCategories))
	for _, category := range selectedCategories {
		clean := strings.TrimSpace(category)
		if clean == "" {
			continue
		}
		query = append(query, "cat:"+clean)
	}
	if len(query) == 0 {
		return nil, fmt.Errorf("at least one non-empty arXiv category is required")
	}

	values := url.Values{}
	values.Set("search_query", strings.Join(query, "+OR+"))
	values.Set("sortBy", "submittedDate")
	values.Set("sortOrder", "descending")
	values.Set("start", "0")
	values.Set("max_results", fmt.Sprintf("%d", limit))

	endpoint := strings.TrimRight(c.BaseURL, "?") + "?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request arXiv API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("arXiv API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(resp.Body)
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
