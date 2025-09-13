// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package main holds sbomgen code
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"

	"github.com/spf13/cobra"
)

func main() {
	var fast bool
	var analyzers []string

	var cpuprofile string
	var closers []io.Closer

	var rootCmd = &cobra.Command{
		Use:   "sbomgen",
		Short: "A generator for SBOMs",
	}
	rootCmd.PersistentFlags().BoolVar(&fast, "fast", false, "use fast mode")
	rootCmd.PersistentFlags().StringSliceVar(&analyzers, "analyzers", nil, "analyzers to use")
	rootCmd.PersistentFlags().StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		if cpuprofile != "" {
			f, err := os.Create(cpuprofile)
			if err != nil {
				return fmt.Errorf("error creating CPU profile: %w", err)
			}
			closers = append(closers, f)
			if err := pprof.StartCPUProfile(f); err != nil {
				return fmt.Errorf("error starting CPU profile: %w", err)
			}
		}
		return nil
	}
	rootCmd.PersistentPostRunE = func(_ *cobra.Command, _ []string) error {
		if cpuprofile != "" {
			pprof.StopCPUProfile()
		}
		for _, c := range closers {
			if err := c.Close(); err != nil {
				return fmt.Errorf("error closing: %w", err)
			}
		}
		return nil
	}

	var removeLayers bool
	var fsCmd = &cobra.Command{
		Use:  "fs",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := args[0]
			return runScanFS(path, analyzers, fast, removeLayers)
		},
	}
	fsCmd.Flags().BoolVar(&removeLayers, "remove-layers", false, "remove layers")
	rootCmd.AddCommand(fsCmd)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
