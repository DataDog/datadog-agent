// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy && containerd && docker && crio

// Package main holds sbomgen code
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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

	var fsCmd = &cobra.Command{
		Use:  "fs",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := args[0]
			return runScanFS(path, analyzers, fast)
		},
	}
	rootCmd.AddCommand(fsCmd)

	var dockerCmd = &cobra.Command{
		Use:  "docker",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			metaPath := args[0]
			imageMeta, err := unmarshalImageMetadata(metaPath)
			if err != nil {
				return err
			}
			return runScanDocker(imageMeta, analyzers, fast)
		},
	}
	rootCmd.AddCommand(dockerCmd)

	var containerdStrategy string
	var containerdCmd = &cobra.Command{
		Use:  "containerd",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			metaPath := args[0]
			imageMeta, err := unmarshalImageMetadata(metaPath)
			if err != nil {
				return err
			}
			return runScanContainerd(imageMeta, analyzers, fast, containerdStrategy)
		},
	}
	containerdCmd.Flags().StringVar(&containerdStrategy, "strategy", "image", "strategy to use (mount, overlayfs or image)")
	rootCmd.AddCommand(containerdCmd)

	var crioCmd = &cobra.Command{
		Use:  "crio",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			metaPath := args[0]
			imageMeta, err := unmarshalImageMetadata(metaPath)
			if err != nil {
				return err
			}
			return runScanCrio(imageMeta, analyzers, fast)
		},
	}
	rootCmd.AddCommand(crioCmd)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func unmarshalImageMetadata(metaPath string) (*workloadmeta.ContainerImageMetadata, error) {
	if metaPath == "" {
		return nil, errors.New("path/image meta is required")
	}

	var metaContent []byte
	if strings.HasPrefix(metaPath, "@") {
		content, err := os.ReadFile(metaPath)
		if err != nil {
			return nil, fmt.Errorf("error reading image metadata: %w", err)
		}
		metaContent = content
	} else {
		metaContent = []byte(metaPath)
	}

	var imageMeta workloadmeta.ContainerImageMetadata
	if err := json.Unmarshal(metaContent, &imageMeta); err != nil {
		return nil, fmt.Errorf("error unmarshalling image metadata: %w", err)
	}

	return &imageMeta, nil
}
