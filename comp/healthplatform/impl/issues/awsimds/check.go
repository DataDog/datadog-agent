// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package awsimds

import (
	"errors"
	"net"
	"os"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

const dialTimeout = 1 * time.Second

// Check detects if the AWS IMDSv2 endpoint is unreachable from inside a container
// due to the default hop limit of 1. The check works by attempting a TCP connection
// to the metadata endpoint: a timeout (as opposed to "no route to host") indicates
// that a route exists but packets are dropped by the EC2 hypervisor because the TTL
// expires after the container-to-host hop.
func Check() (*healthplatform.IssueReport, error) {
	// Only relevant when running inside a container
	if !isContainerized() {
		return nil, nil
	}

	conn, err := net.DialTimeout("tcp", imdsAddress, dialTimeout)
	if err == nil {
		// Connection succeeded - IMDS is reachable, no hop limit issue
		conn.Close()
		return nil, nil
	}

	// A timeout indicates the address is routable (i.e. we are on AWS) but packets
	// are being dropped before reaching the endpoint - the classic hop limit symptom.
	// Other errors (EHOSTUNREACH, ENETUNREACH, ECONNREFUSED) mean IMDS is simply not
	// present on this host, so we do not report an issue.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &healthplatform.IssueReport{
			IssueId: IssueID,
			Context: map[string]string{
				"imds_address": imdsAddress,
			},
			Tags: []string{"aws", "imds", "hop-limit", "container"},
		}, nil
	}

	return nil, nil
}

// isContainerized checks if the agent is running inside a container
func isContainerized() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}
