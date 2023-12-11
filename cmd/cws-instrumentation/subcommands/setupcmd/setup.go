// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package setupcmd holds the setup command of CWS injector
package setupcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/flags"
)

type setupCliParams struct {
	cwsVolumeMount string
}

// Command returns the commands for the setup subcommand
func Command() []*cobra.Command {
	var params setupCliParams

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Copies the cws-instrumentation binary to the CWS volume mount",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setupCWSInjector(&params)
		},
	}

	setupCmd.Flags().StringVar(&params.cwsVolumeMount, flags.CWSVolumeMount, "", "Path to the CWS volume mount")
	_ = setupCmd.MarkFlagRequired(flags.CWSVolumeMount)

	return []*cobra.Command{setupCmd}
}

// setupCWSInjector copies the cws-instrumentation binary to the provided target directory
func setupCWSInjector(params *setupCliParams) error {
	// check if the target directory exists
	targetFileInfo, err := os.Stat(params.cwsVolumeMount)
	if err != nil {
		return fmt.Errorf("couldn't stat target directory: %w", err)
	}
	if !targetFileInfo.IsDir() {
		return fmt.Errorf("\"%s\" must be a directory: %s isn't a valid directory", flags.CWSVolumeMount, params.cwsVolumeMount)
	}

	// fetch the path to the current binary file
	path, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return fmt.Errorf("couldn't resolve the path to the current binary: %w", err)
	}

	// copy the binary to the destination directory
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("couldn't open cws-instrumentation's binary file: %w", err)
	}
	defer source.Close()

	targetPath := filepath.Join(params.cwsVolumeMount, filepath.Base(path))
	target, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("couldn't create target cws-instrumentation binary file in the mounted volume")
	}
	defer target.Close()

	// copy
	_, err = io.Copy(target, source)
	if err != nil {
		return fmt.Errorf("target cws-instrumentation binary couldn't be copied to the mounted volume: %w", err)
	}

	// add execution rights
	if err = os.Chmod(targetPath, 0755); err != nil {
		return fmt.Errorf("couldn't set execution permissions on 'cws-instrumentation': %w", err)
	}
	return nil
}
