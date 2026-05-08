package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fieldwork/config"

	redis "github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

type healthResult struct {
	Name    string
	Status  string
	Details string
}

func newStatusCommand(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show FIELDWORK environment and service health",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			fmt.Fprintln(out, "FIELDWORK status")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Environment")
			fmt.Fprintf(out, "  ARXIV_CATEGORIES: %s\n", strings.Join(cfg.ArxivCategories, ","))
			fmt.Fprintf(out, "  ARXIV_PAPER_LIMIT: %d\n", cfg.ArxivPaperLimit)
			fmt.Fprintf(out, "  MARKER_SERVICE_URL: %s\n", cfg.MarkerServiceURL)
			fmt.Fprintf(out, "  QDRANT_URL: %s\n", cfg.QdrantURL)
			fmt.Fprintf(out, "  REDIS_URL: %s\n", cfg.RedisURL)
			fmt.Fprintf(out, "  PDF_CACHE_DIR: %s\n", cfg.PDFCacheDir)
			fmt.Fprintf(out, "  GEMINI_API_KEY: %s\n", keyStatus(cfg.GeminiAPIKey))
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Service health")

			results := []healthResult{
				healthHTTP("marker", strings.TrimRight(cfg.MarkerServiceURL, "/")+"/health", time.Duration(cfg.MarkerServiceTimeoutSeconds)*time.Second),
				healthHTTP("qdrant", strings.TrimRight(cfg.QdrantURL, "/")+"/collections", 10*time.Second),
				healthRedis(cfg.RedisURL),
			}

			allHealthy := true
			for _, r := range results {
				fmt.Fprintf(out, "  %-8s %-7s %s\n", r.Name, r.Status, r.Details)
				if r.Status != "healthy" {
					allHealthy = false
				}
			}

			if allHealthy {
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "status: healthy")
				return nil
			}

			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "status: degraded")
			return fmt.Errorf("one or more services are unhealthy")
		},
	}
}

func keyStatus(key string) string {
	if strings.TrimSpace(key) == "" {
		return "missing"
	}
	return "set"
}

func healthHTTP(name, url string, timeout time.Duration) healthResult {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return healthResult{Name: name, Status: "down", Details: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return healthResult{Name: name, Status: "healthy", Details: fmt.Sprintf("%s", resp.Status)}
	}

	return healthResult{Name: name, Status: "down", Details: fmt.Sprintf("%s", resp.Status)}
}

func healthRedis(redisURL string) healthResult {
	client := redis.NewClient(&redis.Options{Addr: strings.TrimPrefix(redisURL, "redis://")})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return healthResult{Name: "redis", Status: "down", Details: err.Error()}
	}

	return healthResult{Name: "redis", Status: "healthy", Details: "PONG"}
}
