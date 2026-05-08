package main

import (
	"fmt"

	"fieldwork/config"

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
			fmt.Fprintf(cmd.OutOrStdout(), "dig stub: phase=%s category=%s limit=%d force=%t\n", phase, category, limit, force)
			fmt.Fprintln(cmd.OutOrStdout(), "pipeline implementation begins in milestone 1")
			_ = cfg
			return nil
		},
	}

	cmd.Flags().StringVar(&phase, "phase", "full", "pipeline phase to run")
	cmd.Flags().StringVar(&category, "category", "", "override arXiv category filter")
	cmd.Flags().IntVar(&limit, "limit", cfg.ArxivPaperLimit, "maximum papers to process")
	cmd.Flags().BoolVar(&force, "force", false, "reprocess papers regardless of status")

	return cmd
}
