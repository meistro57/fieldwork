package rss

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

	"fieldwork/internal/arxiv"
)

const (
	defaultBaseURL        = "https://rss.arxiv.org/rss"
	defaultUserAgent      = "fieldwork/1.0 (https://github.com/meistro57/fieldwork)"
	defaultRequestSpacing = 1 * time.Second
)

type Client struct {
	BaseURL         string
	HTTPClient      *http.Client
	UserAgent       string
	RequestInterval time.Duration

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
	}
}

func (c *Client) FetchLatest(ctx context.Context, categories []string, limit int) ([]arxiv.Paper, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}

	selectedCategories := cleanList(categories)
	if len(selectedCategories) == 0 {
		return nil, fmt.Errorf("at least one RSS category is required")
	}

	seen := make(map[string]bool)
	all := make([]arxiv.Paper, 0, limit)
	for _, category := range selectedCategories {
		if err := c.waitForRateLimit(ctx); err != nil {
			return nil, err
		}

		papers, err := c.fetchCategory(ctx, category)
		if err != nil {
			return nil, fmt.Errorf("fetch category %s: %w", category, err)
		}

		for _, paper := range papers {
			id := strings.TrimSpace(paper.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			all = append(all, paper)
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

func (c *Client) fetchCategory(ctx context.Context, category string) ([]arxiv.Paper, error) {
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/" + strings.TrimSpace(category)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if strings.TrimSpace(c.UserAgent) != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request RSS feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("RSS feed returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read RSS response: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse RSS XML: %w", err)
	}

	papers := make([]arxiv.Paper, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		id := extractArxivIDFromGUID(item.GUID)
		if id == "" {
			id = extractArxivIDFromLink(item.Link)
		}
		if id == "" {
			continue
		}

		papers = append(papers, arxiv.Paper{
			ID:          id,
			Title:       collapseWhitespace(item.Title),
			Authors:     parseAuthors(item.DCCreator, item.Creator),
			Abstract:    collapseWhitespace(item.Description),
			Categories:  cleanList(item.Categories),
			SubmittedAt: parsePubDate(item.PubDate),
			PDFURL:      toPDFURL(id),
		})
	}

	return papers, nil
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

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	DCCreator   string   `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Creator     string   `xml:"creator"`
	PubDate     string   `xml:"pubDate"`
	Categories  []string `xml:"category"`
	GUID        string   `xml:"guid"`
}

func extractArxivIDFromGUID(guid string) string {
	trimmed := strings.TrimSpace(guid)
	trimmed = strings.TrimPrefix(trimmed, "oai:arXiv.org:")
	return strings.TrimSpace(trimmed)
}

func extractArxivIDFromLink(link string) string {
	trimmed := strings.TrimSpace(link)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err == nil {
		id := strings.TrimSpace(path.Base(parsed.Path))
		if id != "" && id != "/" {
			return id
		}
	}

	if i := strings.LastIndex(trimmed, "/"); i >= 0 && i+1 < len(trimmed) {
		return strings.TrimSpace(trimmed[i+1:])
	}

	return ""
}

func toPDFURL(id string) string {
	clean := strings.TrimSpace(id)
	if clean == "" {
		return ""
	}
	return "https://arxiv.org/pdf/" + clean
}

func parsePubDate(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}
	}

	formats := []string{time.RFC1123Z, time.RFC1123}
	for _, format := range formats {
		if parsed, err := time.Parse(format, trimmed); err == nil {
			return parsed
		}
	}

	return time.Time{}
}

func parseAuthors(dcCreator, creator string) []string {
	raw := strings.TrimSpace(dcCreator)
	if raw == "" {
		raw = strings.TrimSpace(creator)
	}
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, sub := range strings.Split(part, " and ") {
			author := strings.TrimSpace(sub)
			if author != "" {
				out = append(out, author)
			}
		}
	}

	if len(out) == 0 {
		return []string{raw}
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

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
