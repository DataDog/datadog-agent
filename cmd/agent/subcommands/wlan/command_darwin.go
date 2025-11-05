// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package wlan implements 'agent wlan'.
package wlan

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/wlan"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	wlanCmd := &cobra.Command{
		Use:   "wlan [command]",
		Short: "WLAN integration commands",
		Long:  "Commands to manage the WLAN integration on macOS",
	}

	requestPermissionCmd := &cobra.Command{
		Use:   "request-location-permission",
		Short: "Request location permission for WiFi monitoring",
		Long: `Launches a GUI prompt requesting location services permission.

This permission is required to collect WiFi SSID and BSSID information on macOS Big Sur and later.
Without this permission, the WLAN check will still collect signal strength (RSSI), noise, and data rates.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("🚀 Requesting location permission for WiFi monitoring...")
			fmt.Println("   Please respond to the system dialog that will appear.")
			fmt.Println()

			// This blocks until the user responds or timeout (30s)
			wlan.RequestLocationPermissionGUI()

			return nil
		},
	}

	wlanCmd.AddCommand(requestPermissionCmd)
	return []*cobra.Command{wlanCmd}
}
