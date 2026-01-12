//go:build ec2

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package creds

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const (
	// imdsBaseURL is the EC2 instance metadata service base URL
	imdsBaseURL = "http://169.254.169.254"
	// imdsTimeout is the timeout for IMDS requests (300ms matches the typical config)
	imdsTimeout = 300 * time.Millisecond
)

// instanceIdentity represents the EC2 instance identity document
type instanceIdentity struct {
	Region     string `json:"region"`
	InstanceID string `json:"instanceId"`
}

// IsRunningOnAWS returns true if the code is running on an AWS EC2 instance.
// This is a lightweight check that doesn't depend on agent configuration.
func IsRunningOnAWS(ctx context.Context) bool {
	client := &http.Client{
		Timeout: imdsTimeout,
	}

	// Try to fetch the instance identity document from IMDS
	// Using the instance-identity document endpoint which is reliable
	req, err := http.NewRequestWithContext(ctx, "GET", imdsBaseURL+"/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetAWSRegion returns the AWS region for the current EC2 instance.
// Returns empty string and error if not running on AWS or if region cannot be determined.
func GetAWSRegion(ctx context.Context) (string, error) {
	client := &http.Client{
		Timeout: imdsTimeout,
	}

	// Get region from the instance identity document
	// This is more reliable than the placement/region endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", imdsBaseURL+"/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var identity instanceIdentity
	if err := json.Unmarshal(body, &identity); err != nil {
		return "", err
	}

	return identity.Region, nil
}
