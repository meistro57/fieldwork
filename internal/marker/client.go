package marker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Retries    int
	Backoff    time.Duration
}

type ParseResult struct {
	Markdown    string   `json:"markdown"`
	Title       string   `json:"title"`
	Sections    []string `json:"sections"`
	Charts      []Chart  `json:"charts"`
	LatexBlocks []string `json:"latex_blocks"`
}

type Chart struct {
	Index         int    `json:"index"`
	ImageB64      string `json:"image_b64"`
	ContextBefore string `json:"context_before"`
	ContextAfter  string `json:"context_after"`
	Caption       string `json:"caption"`
}

type parseRequest struct {
	PDFPath string `json:"pdf_path"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
		Retries: 2,
		Backoff: 5 * time.Second,
	}
}

func (c *Client) Parse(ctx context.Context, pdfPath string) (*ParseResult, error) {
	if strings.TrimSpace(pdfPath) == "" {
		return nil, fmt.Errorf("pdf path cannot be empty")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("marker base URL cannot be empty")
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 120 * time.Second}
	}
	if c.Retries <= 0 {
		c.Retries = 2
	}
	if c.Backoff <= 0 {
		c.Backoff = 5 * time.Second
	}

	payload, err := json.Marshal(parseRequest{PDFPath: pdfPath})
	if err != nil {
		return nil, fmt.Errorf("marshal parse request: %w", err)
	}

	attempts := c.Retries
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := c.parseOnce(ctx, payload)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if attempt == attempts {
			break
		}

		timer := time.NewTimer(c.Backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
}

func (c *Client) parseOnce(ctx context.Context, payload []byte) (*ParseResult, error) {
	endpoint := c.BaseURL + "/parse"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create marker request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call marker parse endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("marker parse returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out ParseResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode marker parse response: %w", err)
	}

	return &out, nil
}
