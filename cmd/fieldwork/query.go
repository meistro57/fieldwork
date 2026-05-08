package main

import (
	"fmt"

	"fieldwork/config"

	"github.com/spf13/cobra"
)

func newQueryCommand(cfg *config.Config) *cobra.Command {
	var (
		vector    string
		limit     int
		category  string
		pointType string
		jsonOut   bool
	)

	cmd := &cobra.Command{
		Use:   "query <text>",
		Short: "Semantic query interface for FIELDWORK",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "query stub: q=%q vector=%s limit=%d category=%s type=%s json=%t\n", args[0], vector, limit, category, pointType, jsonOut)
			fmt.Fprintln(cmd.OutOrStdout(), "query implementation begins in milestone 6")
			_ = cfg
			return nil
		},
	}

	cmd.Flags().StringVar(&vector, "vector", "all", "named vector to search (text|abstract|chart|all)")
	cmd.Flags().IntVar(&limit, "limit", 5, "maximum results")
	cmd.Flags().StringVar(&category, "category", "", "optional arXiv category filter")
	cmd.Flags().StringVar(&pointType, "type", "", "optional point type filter")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON output")

	return cmd
}
