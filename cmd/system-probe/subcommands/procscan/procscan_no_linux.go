// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux_bpf

// Package procscan implements the 'procscan' subcommand that emulates the
// behavior of the Go process scanner.
package procscan

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(*command.GlobalParams) []*cobra.Command {
	procscanCommand := &cobra.Command{
		Use:   "procscan",
		Short: "Scan the system for Go processes that use the Datadog tracer library. Unsupported on this platform.",
		Long:  ``,
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			fmt.Println("procscan is not supported on this platform.")
			return nil
		},
	}

	return []*cobra.Command{procscanCommand}
}
