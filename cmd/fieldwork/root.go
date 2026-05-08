package main

import (
	"fieldwork/config"

	"github.com/spf13/cobra"
)

func newRootCommand(cfg *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "fieldwork",
		Short: "FIELDWORK CLI",
		Long:  "FIELDWORK excavates and queries arXiv physics pre-prints.",
	}

	rootCmd.AddCommand(newDigCommand(cfg))
	rootCmd.AddCommand(newQueryCommand(cfg))
	rootCmd.AddCommand(newStatusCommand(cfg))

	return rootCmd
}
