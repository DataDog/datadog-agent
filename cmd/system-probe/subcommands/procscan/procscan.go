// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package procscan implements the 'procscan' subcommand that emulates the
// behavior of the Go process scanner.
package procscan

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe/procscan"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type cliParams struct {
	*command.GlobalParams

	// args contains the positional command-line arguments.
	args []string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	procscanCommand := &cobra.Command{
		Use:   "procscan",
		Short: "Scan the system for Go processes that use the Datadog tracer library",
		Long:  ``,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.args = args
			return scanForProcesses()
		},
	}

	return []*cobra.Command{procscanCommand}
}

func scanForProcesses() error {
	// Create a scanner to discover processes.
	scanner := procscan.NewScanner(kernel.ProcFSRoot(), 0 /* startDelay */)

	fmt.Println("Scanning for Go processes...")
	fmt.Println()

	// Perform the scan.
	newProcesses, removedProcesses, err := scanner.Scan()
	if err != nil {
		return fmt.Errorf("failed to scan processes: %w", err)
	}
	if len(removedProcesses) > 0 {
		fmt.Printf("Expected removedProcesses to be empty, but found %d process(es):\n", len(removedProcesses))
		for _, pid := range removedProcesses {
			fmt.Printf("  PID: %d\n", pid)
		}
	}

	// Print discovered processes.
	if len(newProcesses) == 0 {
		fmt.Println("No Go processes with the Datadog tracer found.")
	} else {
		fmt.Printf("Found %d Go process(es) with Dynamic Instrumentation enabled:\n", len(newProcesses))
		fmt.Println()
		for i, proc := range newProcesses {
			fmt.Printf("Process #%d:\n", i+1)
			fmt.Printf("  PID:              %d\n", proc.PID)
			fmt.Printf("  RuntimeID:        %s\n", proc.RuntimeID)
			fmt.Printf("  Start Time Ticks: %d\n", proc.StartTimeTicks)
			fmt.Printf("  Executable Path:  %s\n", proc.Executable.Path)
			fmt.Printf("  File Key:         %s\n", proc.Executable.Key)
			fmt.Printf("  Language:         %s\n", proc.TracerLanguage)
			fmt.Printf("  Service Name:     %s\n", proc.ServiceName)
			fmt.Printf("  Service Version:  %s\n", proc.ServiceVersion)
			fmt.Println()
		}
	}

	return nil
}
