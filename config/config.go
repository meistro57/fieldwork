package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	ArxivCategories               []string
	ArxivPaperLimit               int
	ArxivRateLimitSeconds         int
	GeminiAPIKey                  string
	GeminiEmbeddingModel          string
	GeminiEmbeddingDimensions     int
	MarkerServiceURL              string
	MarkerServiceTimeoutSeconds   int
	QdrantURL                     string
	QdrantCollection              string
	RedisURL                      string
	RedisCacheTTLHours            int
	RedisCacheSimilarityThreshold float64
	PipelineWorkers               int
	PDFCacheDir                   string
	MarkerPDFRoot                 string
	LogLevel                      string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		ArxivCategories:               getCSV("ARXIV_CATEGORIES", "quant-ph,gr-qc,hep-th,hep-ph,math-ph,cond-mat"),
		ArxivPaperLimit:               getInt("ARXIV_PAPER_LIMIT", 50),
		ArxivRateLimitSeconds:         getInt("ARXIV_RATE_LIMIT_SECONDS", 3),
		GeminiAPIKey:                  strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GeminiEmbeddingModel:          getString("GEMINI_EMBEDDING_MODEL", "text-embedding-004"),
		GeminiEmbeddingDimensions:     getInt("GEMINI_EMBEDDING_DIMENSIONS", 3072),
		MarkerServiceURL:              getString("MARKER_SERVICE_URL", "http://localhost:8000"),
		MarkerServiceTimeoutSeconds:   getInt("MARKER_SERVICE_TIMEOUT_SECONDS", 120),
		QdrantURL:                     getString("QDRANT_URL", "http://localhost:6333"),
		QdrantCollection:              getString("QDRANT_COLLECTION", "fieldwork"),
		RedisURL:                      getString("REDIS_URL", "redis://localhost:6379"),
		RedisCacheTTLHours:            getInt("REDIS_CACHE_TTL_HOURS", 24),
		RedisCacheSimilarityThreshold: getFloat("REDIS_CACHE_SIMILARITY_THRESHOLD", 0.92),
		PipelineWorkers:               getInt("PIPELINE_WORKERS", 3),
		PDFCacheDir:                   getString("PDF_CACHE_DIR", "./data/pdfs"),
		MarkerPDFRoot:                 getString("MARKER_PDF_ROOT", ""),
		LogLevel:                      getString("LOG_LEVEL", "info"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	var errs []string

	if len(c.ArxivCategories) == 0 {
		errs = append(errs, "ARXIV_CATEGORIES must include at least one category")
	}
	if c.ArxivPaperLimit <= 0 {
		errs = append(errs, "ARXIV_PAPER_LIMIT must be > 0")
	}
	if c.ArxivRateLimitSeconds <= 0 {
		errs = append(errs, "ARXIV_RATE_LIMIT_SECONDS must be > 0")
	}
	if c.GeminiEmbeddingDimensions <= 0 {
		errs = append(errs, "GEMINI_EMBEDDING_DIMENSIONS must be > 0")
	}
	if strings.TrimSpace(c.MarkerServiceURL) == "" {
		errs = append(errs, "MARKER_SERVICE_URL cannot be empty")
	}
	if c.MarkerServiceTimeoutSeconds <= 0 {
		errs = append(errs, "MARKER_SERVICE_TIMEOUT_SECONDS must be > 0")
	}
	if strings.TrimSpace(c.QdrantURL) == "" {
		errs = append(errs, "QDRANT_URL cannot be empty")
	}
	if strings.TrimSpace(c.QdrantCollection) == "" {
		errs = append(errs, "QDRANT_COLLECTION cannot be empty")
	}
	if strings.TrimSpace(c.RedisURL) == "" {
		errs = append(errs, "REDIS_URL cannot be empty")
	}
	if c.RedisCacheTTLHours <= 0 {
		errs = append(errs, "REDIS_CACHE_TTL_HOURS must be > 0")
	}
	if c.RedisCacheSimilarityThreshold <= 0 || c.RedisCacheSimilarityThreshold > 1 {
		errs = append(errs, "REDIS_CACHE_SIMILARITY_THRESHOLD must be > 0 and <= 1")
	}
	if c.PipelineWorkers <= 0 {
		errs = append(errs, "PIPELINE_WORKERS must be > 0")
	}
	if strings.TrimSpace(c.PDFCacheDir) == "" {
		errs = append(errs, "PDF_CACHE_DIR cannot be empty")
	}
	if strings.TrimSpace(c.LogLevel) == "" {
		errs = append(errs, "LOG_LEVEL cannot be empty")
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration:\n- %s", strings.Join(errs, "\n- "))
	}

	return nil
}

func getString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getCSV(key, fallback string) []string {
	raw := getString(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		clean := strings.TrimSpace(p)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}
