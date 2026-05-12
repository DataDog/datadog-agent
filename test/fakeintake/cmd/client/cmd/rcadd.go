// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewRCAddCommand returns the `rc add` subcommand.
func NewRCAddCommand(cl **client.Client) *cobra.Command {
	var orgID, product, configID, configName, dataArg string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Push a Remote Config entry to fakeintake",
		Example: `  # literal JSON
  fakeintakectl --url http://localhost:80 rc add --product METRIC_CONTROL \
      --config-id abc --config-name fl --data '{"blocked_metrics":{}}'

  # JSON from a file
  fakeintakectl --url http://localhost:80 rc add --product METRIC_CONTROL \
      --config-id abc --config-name fl --data @config.json

  # JSON from stdin
  cat config.json | fakeintakectl --url http://localhost:80 rc add \
      --product METRIC_CONTROL --config-id abc --config-name fl --data -`,
		RunE: func(_ *cobra.Command, _ []string) error {
			data, err := readData(dataArg)
			if err != nil {
				return err
			}
			if !json.Valid(data) {
				return errors.New("--data is not valid JSON")
			}
			return (*cl).RCAddConfig(orgID, product, configID, configName, data)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "org ID (defaults to fakeintake's configured org UUID)")
	cmd.Flags().StringVar(&product, "product", "", "Remote Config product (e.g. METRIC_CONTROL)")
	cmd.Flags().StringVar(&configID, "config-id", "", "config ID")
	cmd.Flags().StringVar(&configName, "config-name", "", "config name")
	cmd.Flags().StringVar(&dataArg, "data", "", "JSON config payload: literal, @file, or - for stdin")
	for _, name := range []string{"product", "config-id", "config-name", "data"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func readData(arg string) ([]byte, error) {
	switch {
	case arg == "-":
		return io.ReadAll(os.Stdin)
	case strings.HasPrefix(arg, "@"):
		return os.ReadFile(strings.TrimPrefix(arg, "@"))
	default:
		return []byte(arg), nil
	}
}
