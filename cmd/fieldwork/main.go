package main

import (
	"fmt"
	"os"

	"fieldwork/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	rootCmd := newRootCommand(cfg)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "command failed: %v\n", err)
		os.Exit(1)
	}
}
