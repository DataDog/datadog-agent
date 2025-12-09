// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privateactionrunner implements the private action runner CLI subcommand
package privateactionrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/config/create"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// Commands returns the private action runner subcommands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "private-action-runner",
		Short: "Private Action Runner commands",
		Long: `Private Action Runner allows the Datadog Agent to execute approved actions
securely within customer environments. Use these commands to manage enrollment
and configuration.`,
	}

	cmd.AddCommand(selfEnrollCommand(globalParams))

	return []*cobra.Command{cmd}
}

func selfEnrollCommand(_ *command.GlobalParams) *cobra.Command {
	var apiKey string
	var appKey string
	var site string
	var runnerName string
	var actionsAllowList string
	var connectionGroupID string

	cmd := &cobra.Command{
		Use:   "self-enroll --api-key <api-key> --app-key <app-key>",
		Short: "Self-enroll this agent as a private action runner using API / App key authentication",
		Long: `Self-enroll this agent as a private action runner using API / App key authentication.

This command generates a new public/private key pair and sends the public key to the
self-enroll OPMS endpoint. The enrollment configuration will be printed to stdout.

Example:
  datadog-agent private-action-runner self-enroll --api-key "your-api-key"
  datadog-agent private-action-runner self-enroll --api-key "key" --site datadoghq.eu
`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if appKey == "" {
				appKey = os.Getenv("DD_APP_KEY")
			}
			if apiKey == "" {
				apiKey = os.Getenv("DD_API_KEY")
			}
			if apiKey == "" {
				return errors.New("API key is required. Use --api-key flag")
			}
			if appKey == "" {
				return errors.New("App key is required. Use --app-key flag")
			}
			if site == "" {
				site = "datadoghq.com" // Default site
			}
			if runnerName == "" {
				// Initialize minimal config and feature detection for hostname resolution
				cfg := create.NewConfig("datadog")
				env.DetectFeatures(cfg)

				if agentHostname, err := hostname.Get(context.Background()); err == nil {
					runnerName = agentHostname
				} else {
					runnerName = fmt.Sprintf("agent_%d", time.Now().Unix())
				}
			}
			// Perform self-enrollment
			return enrollment.ProvisionRunnerIdentityWithAPIKey(apiKey, appKey, site, runnerName, actionsAllowList, connectionGroupID)
		},
	}

	cmd.Flags().StringVarP(&apiKey, "api-key", "", "", "Datadog API key for authentication (required)")
	cmd.Flags().StringVarP(&appKey, "app-key", "", "", "Datadog APP key for authentication (required)")
	cmd.Flags().StringVarP(&site, "site", "", "", "Datadog site (e.g., datadoghq.com, datadoghq.eu, us3.datadoghq.com). Defaults to datadoghq.com")
	cmd.Flags().StringVarP(&runnerName, "name", "", "", "Name of the private action runner")
	cmd.Flags().StringVarP(&actionsAllowList, "actions-allowlist", "", "com.datadoghq.datadog.agentactions.helloWorld", "Allowlist of actions for the private action runner")
	cmd.Flags().StringVarP(&connectionGroupID, "connection-group-id", "", "", "Join a connection group on creation")
	//cmd.MarkFlagRequired("api-key")
	//cmd.MarkFlagRequired("app-key")

	return cmd
}
