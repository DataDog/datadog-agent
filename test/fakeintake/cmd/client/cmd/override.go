// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewOverrideCommand returns the override command
func NewOverrideCommand(cl **client.Client) *cobra.Command {
	var endpoint, method, contentType, bodyArg string
	var statusCode int

	cmd := &cobra.Command{
		Use:   "override",
		Short: "Configure a response override for a fakeintake endpoint",
		Example: `  # Return HTTP 500 for all POST requests to /api/v2/series
  fakeintakectl --url http://localhost:80 override \
      --endpoint /api/v2/series --status-code 500 --method POST

  # Return a custom body from a file
  fakeintakectl --url http://localhost:80 override \
      --endpoint /api/v2/logs --status-code 200 --body @response.json

  # Return a custom body from stdin
  echo '{"status":"ok"}' | fakeintakectl --url http://localhost:80 override \
      --endpoint /api/v2/logs --status-code 200 --body -`,
		RunE: func(_ *cobra.Command, _ []string) error {
			var body []byte
			if bodyArg != "" {
				var err error
				body, err = readData(bodyArg)
				if err != nil {
					return err
				}
			}
			return (*cl).ConfigureOverride(api.ResponseOverride{
				Endpoint:    endpoint,
				StatusCode:  statusCode,
				ContentType: contentType,
				Method:      method,
				Body:        body,
			})
		},
	}

	cmd.Flags().StringVar(&endpoint, "endpoint", "", "endpoint path to override (e.g. /api/v2/series)")
	cmd.Flags().IntVar(&statusCode, "status-code", 0, "HTTP status code to return")
	cmd.Flags().StringVar(&method, "method", "", "HTTP method to match (e.g. POST); empty matches all methods")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Content-Type header for the override response")
	cmd.Flags().StringVar(&bodyArg, "body", "", "response body: literal string, @file, or - for stdin")

	for _, name := range []string{"endpoint", "status-code"} {
		_ = cmd.MarkFlagRequired(name)
	}

	return cmd
}
