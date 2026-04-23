// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package backendconnectivity

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

const (
	defaultSite = "datadoghq.com"
	dialTimeout = 5 * time.Second
)

// Check checks if the agent can reach the configured Datadog backend endpoints.
// It attempts a TCP dial to the intake endpoint derived from the site config,
// and optionally to a custom dd_url if configured.
func Check(cfg config.Component) (*healthplatform.IssueReport, error) {
	site := cfg.GetString("site")
	if site == "" {
		site = defaultSite
	}

	endpoint := fmt.Sprintf("app.%s:443", site)
	if err := dialEndpoint(endpoint); err != nil {
		return &healthplatform.IssueReport{
			IssueId: IssueID,
			Context: map[string]string{
				"endpoint": endpoint,
				"error":    err.Error(),
			},
		}, nil
	}

	// Also check dd_url if explicitly configured
	ddURL := cfg.GetString("dd_url")
	if ddURL != "" {
		urlEndpoint := fmt.Sprintf("%s:443", ddURL)
		if err := dialEndpoint(urlEndpoint); err != nil {
			return &healthplatform.IssueReport{
				IssueId: IssueID,
				Context: map[string]string{
					"endpoint": urlEndpoint,
					"error":    err.Error(),
				},
			}, nil
		}
	}

	// No connectivity issue detected
	return nil, nil
}

// dialEndpoint attempts a TCP connection to the given address with a timeout.
func dialEndpoint(address string) error {
	conn, err := net.DialTimeout("tcp", address, dialTimeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
