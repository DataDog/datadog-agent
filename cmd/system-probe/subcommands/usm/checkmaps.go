// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/network/usm/maps"
)

func makeCheckMapsCommand(_ *command.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "check-maps",
		Short: "Check USM eBPF maps for leaked entries",
		Long: `Check USM eBPF maps for leaked entries by validating map keys against system state.

For PID-keyed maps (TLS/SSL argument storage), this command:
  - Extracts PIDs from map keys
  - Checks if processes still exist in /proc
  - Reports entries where the process no longer exists (leaked entries)

This is useful for diagnosing memory leaks in customer environments where
eBPF map entries are not being properly cleaned up.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCheckMaps()
		},
	}
}

func runCheckMaps() error {
	report, err := maps.CheckPIDKeyedMaps()
	if err != nil {
		return fmt.Errorf("failed to check maps: %w", err)
	}

	if report.TotalMapsChecked == 0 {
		fmt.Println("No USM eBPF maps found. Is system-probe running with USM enabled?")
		return nil
	}

	// Print the report
	fmt.Print(report.String())

	return nil
}
