package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fieldwork/config"
	"fieldwork/internal/arxiv"
	"fieldwork/internal/harvester"
	"fieldwork/internal/marker"
	"fieldwork/internal/store"

	"github.com/spf13/cobra"
)

func newDigCommand(cfg *config.Config) *cobra.Command {
	var (
		phase    string
		category string
		limit    int
		force    bool
	)

	cmd := &cobra.Command{
		Use:   "dig",
		Short: "Run the FIELDWORK ingestion pipeline",
		RunE: func(cmd *cobra.Command, args []string) error {
			if phase != "harvest" && phase != "full" {
				return fmt.Errorf("unsupported phase %q (allowed: harvest, full)", phase)
			}

			selectedCategories := cfg.ArxivCategories
			if category != "" {
				selectedCategories = []string{category}
			}

			redisStore, err := store.NewRedisStore(cfg.RedisURL)
			if err != nil {
				return err
			}
			defer redisStore.Close()

			arxivClient := arxiv.NewClient(cfg.ArxivCategories, nil)
			h := harvester.New(
				arxivClient,
				redisStore,
				cfg.PDFCacheDir,
				time.Duration(cfg.ArxivRateLimitSeconds)*time.Second,
			)

			harvestResult, err := h.Harvest(cmd.Context(), selectedCategories, limit)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "harvest complete: total=%d downloaded=%d skipped=%d failed=%d force=%t\n", harvestResult.Total, harvestResult.Downloaded, harvestResult.Skipped, harvestResult.Failed, force)
			if phase == "harvest" {
				return nil
			}

			markerClient := marker.NewClient(cfg.MarkerServiceURL, time.Duration(cfg.MarkerServiceTimeoutSeconds)*time.Second)
			downloadedIDs, err := redisStore.ListByStatus(cmd.Context(), "downloaded")
			if err != nil {
				return fmt.Errorf("list downloaded papers from redis: %w", err)
			}

			parsedCount := 0
			parseFailed := 0
			for _, id := range downloadedIDs {
				cleanID := strings.TrimSpace(id)
				if cleanID == "" {
					continue
				}

				pdfPath := filepath.Join(cfg.PDFCacheDir, cleanID+".pdf")
				if _, err := markerClient.Parse(cmd.Context(), pdfPath); err != nil {
					parseFailed++
					_ = redisStore.SetStatus(cmd.Context(), cleanID, "failed")
					continue
				}

				if err := redisStore.SetStatus(cmd.Context(), cleanID, "parsed"); err != nil {
					parseFailed++
					continue
				}
				parsedCount++
			}

			_ = redisStore.SetDigStats(cmd.Context(), map[string]any{
				"total":      harvestResult.Total,
				"downloaded": harvestResult.Downloaded,
				"parsed":     parsedCount,
				"embedded":   0,
				"done":       0,
				"failed":     harvestResult.Failed + parseFailed,
			})

			fmt.Fprintf(cmd.OutOrStdout(), "parse complete: parsed=%d failed=%d\n", parsedCount, parseFailed)
			return nil
		},
	}

	cmd.Flags().StringVar(&phase, "phase", "full", "pipeline phase to run")
	cmd.Flags().StringVar(&category, "category", "", "override arXiv category filter")
	cmd.Flags().IntVar(&limit, "limit", cfg.ArxivPaperLimit, "maximum papers to process")
	cmd.Flags().BoolVar(&force, "force", false, "reprocess papers regardless of status")

	return cmd
}
