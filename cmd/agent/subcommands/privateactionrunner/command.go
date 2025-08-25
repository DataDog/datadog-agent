// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privateactionrunner implements the private action runner CLI subcommand
package privateactionrunner

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/spf13/cobra"
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

	cmd.AddCommand(enrollCommand(globalParams))
	cmd.AddCommand(selfEnrollCommand(globalParams))

	return []*cobra.Command{cmd}
}

func enrollCommand(globalParams *command.GlobalParams) *cobra.Command {
	var enrollmentToken string
	var site string

	cmd := &cobra.Command{
		Use:   "enroll --token <enrollment-token>",
		Short: "Enroll this agent as a private action runner",
		Long: `Enroll this agent as a private action runner with Datadog.

This command requires an enrollment token that can be obtained from the Datadog UI.
The enrollment configuration will be printed to stdout.

Example:
  datadog-agent private-action-runner enroll --token "your-enrollment-token"
  datadog-agent private-action-runner enroll --token "token" --site datadoghq.eu
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if enrollmentToken == "" {
				return fmt.Errorf("enrollment token is required. Use --token flag")
			}
			if site == "" {
				site = "datadoghq.com" // Default site
			}
			// Perform enrollment
			return enrollment.ProvisionRunnerIdentityWithToken(enrollmentToken, site, "")
		},
	}

	cmd.Flags().StringVarP(&enrollmentToken, "token", "t", "", "Enrollment token from Datadog UI (required)")
	cmd.Flags().StringVarP(&site, "site", "s", "", "Datadog site (e.g., datadoghq.com, datadoghq.eu, us3.datadoghq.com). Defaults to datadoghq.com")
	cmd.MarkFlagRequired("token")

	return cmd
}

func selfEnrollCommand(globalParams *command.GlobalParams) *cobra.Command {
	var apiKey string
	var appKey string
	var site string

	cmd := &cobra.Command{
		Use:   "self-enroll --api-key <api-key>",
		Short: "Self-enroll this agent as a private action runner using API key authentication",
		Long: `Self-enroll this agent as a private action runner using API key authentication.

This command generates a new public/private key pair and sends the public key to the
self-enroll OPMS endpoint. The enrollment configuration will be printed to stdout.

Example:
  datadog-agent private-action-runner self-enroll --api-key "your-api-key"
  datadog-agent private-action-runner self-enroll --api-key "key" --site datadoghq.eu
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if appKey == "" {
				appKey = os.Getenv("DD_APP_KEY")
			}
			if apiKey == "" {
				apiKey = os.Getenv("DD_API_KEY")
			}
			if apiKey == "" {
				return fmt.Errorf("API key is required. Use --api-key flag")
			}
			if appKey == "" {
				return fmt.Errorf("App key is required. Use --app-key flag")
			}
			if site == "" {
				site = "datadoghq.com" // Default site
			}
			// Perform self-enrollment
			return enrollment.ProvisionRunnerIdentityWithAPIKey(apiKey, appKey, site)
		},
	}

	cmd.Flags().StringVarP(&apiKey, "api-key", "k", "", "Datadog API key for authentication (required)")
	cmd.Flags().StringVarP(&appKey, "app-key", "", "", "Datadog APP key for authentication (required)")
	cmd.Flags().StringVarP(&site, "site", "s", "", "Datadog site (e.g., datadoghq.com, datadoghq.eu, us3.datadoghq.com). Defaults to datadoghq.com")
	//cmd.MarkFlagRequired("api-key")
	//cmd.MarkFlagRequired("app-key")

	return cmd
}
