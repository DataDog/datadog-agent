// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package awshostnamemismatch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

const (
	imdsInstanceIDURL = "http://169.254.169.254/latest/meta-data/instance-id"
	imdsTimeout       = 1 * time.Second
)

// Check detects whether the agent's configured hostname matches the EC2 instance ID.
// Returns nil if the agent is not on AWS or if no manual hostname is configured.
func Check(cfg config.Component) (*healthplatform.IssueReport, error) {
	// Only fire when a hostname is explicitly configured; auto-detected hostnames are resolved separately.
	configuredHostname := cfg.GetString("hostname")
	if configuredHostname == "" {
		return nil, nil
	}

	instanceID, err := getInstanceID()
	if err != nil {
		// Not on AWS or IMDS unreachable — not our problem.
		return nil, nil
	}

	if strings.Contains(configuredHostname, instanceID) {
		return nil, nil
	}

	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"configuredHostname": configuredHostname,
			"ec2InstanceId":      instanceID,
		},
	}, nil
}

// getInstanceID fetches the EC2 instance ID from IMDS.
// Returns an error if IMDS is unreachable (e.g. not on AWS, timeout, connection refused).
func getInstanceID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), imdsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imdsInstanceIDURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: imdsTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}
