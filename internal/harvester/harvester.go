package harvester

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fieldwork/internal/arxiv"
	"fieldwork/internal/store"
)

type PaperSource interface {
	FetchLatest(ctx context.Context, categories []string, limit int) ([]arxiv.Paper, error)
}

type Harvester struct {
	PaperClient PaperSource
	Store       *store.RedisStore
	PDFCacheDir string
	RateLimit   time.Duration
	HTTPClient  *http.Client

	lastDownload time.Time
}

type Result struct {
	Total      int
	Downloaded int
	Skipped    int
	Failed     int
}

func New(paperClient PaperSource, redisStore *store.RedisStore, pdfCacheDir string, rateLimit time.Duration) *Harvester {
	return &Harvester{
		PaperClient: paperClient,
		Store:       redisStore,
		PDFCacheDir: pdfCacheDir,
		RateLimit:   rateLimit,
		HTTPClient:  &http.Client{Timeout: 2 * time.Minute},
	}
}

func (h *Harvester) Harvest(ctx context.Context, categories []string, limit int) (*Result, error) {
	if h.PaperClient == nil {
		return nil, fmt.Errorf("paper client is required")
	}
	if h.Store == nil {
		return nil, fmt.Errorf("redis store is required")
	}
	if strings.TrimSpace(h.PDFCacheDir) == "" {
		return nil, fmt.Errorf("pdf cache dir is required")
	}
	if h.RateLimit <= 0 {
		return nil, fmt.Errorf("rate limit must be greater than zero")
	}
	if h.HTTPClient == nil {
		h.HTTPClient = &http.Client{Timeout: 2 * time.Minute}
	}

	papers, err := h.PaperClient.FetchLatest(ctx, categories, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch latest papers: %w", err)
	}

	if err := os.MkdirAll(h.PDFCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create PDF cache directory: %w", err)
	}

	runTimestamp := time.Now().UTC().Format(time.RFC3339)
	if err := h.Store.SetDigCurrent(ctx, runTimestamp); err != nil {
		return nil, fmt.Errorf("set current dig timestamp: %w", err)
	}

	result := &Result{Total: len(papers)}
	for _, paper := range papers {
		id := strings.TrimSpace(paper.ID)
		if id == "" {
			result.Failed++
			continue
		}

		pdfPath := filepath.Join(h.PDFCacheDir, id+".pdf")
		exists, err := fileExists(pdfPath)
		if err != nil {
			result.Failed++
			_ = h.Store.SetStatus(ctx, id, "failed")
			continue
		}

		if !exists {
			if err := h.waitForRateLimit(ctx); err != nil {
				return nil, err
			}

			if err := h.downloadPDF(ctx, paper.PDFURL, pdfPath); err != nil {
				result.Failed++
				_ = h.Store.SetStatus(ctx, id, "failed")
				continue
			}
			result.Downloaded++
		} else {
			result.Skipped++
		}

		if err := h.Store.SetMeta(ctx, id, paper); err != nil {
			result.Failed++
			_ = h.Store.SetStatus(ctx, id, "failed")
			continue
		}
		if err := h.Store.SetStatus(ctx, id, "downloaded"); err != nil {
			result.Failed++
			continue
		}
	}

	_ = h.Store.SetDigStats(ctx, map[string]any{
		"total":      result.Total,
		"downloaded": result.Downloaded,
		"parsed":     0,
		"embedded":   0,
		"done":       0,
		"failed":     result.Failed,
	})

	return result, nil
}

func (h *Harvester) waitForRateLimit(ctx context.Context) error {
	wait := h.RateLimit
	if !h.lastDownload.IsZero() {
		nextAllowed := h.lastDownload.Add(h.RateLimit)
		wait = time.Until(nextAllowed)
	}

	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	h.lastDownload = time.Now()
	return nil
}

func (h *Harvester) downloadPDF(ctx context.Context, pdfURL, destination string) error {
	if strings.TrimSpace(pdfURL) == "" {
		return fmt.Errorf("pdf URL cannot be empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pdfURL, nil)
	if err != nil {
		return fmt.Errorf("create PDF request: %w", err)
	}

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download PDF: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download PDF status: %s", resp.Status)
	}

	tmpPath := destination + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp PDF file: %w", err)
	}

	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write PDF file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close PDF file: %w", closeErr)
	}

	if err := os.Rename(tmpPath, destination); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize PDF file: %w", err)
	}

	return nil
}

func fileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
