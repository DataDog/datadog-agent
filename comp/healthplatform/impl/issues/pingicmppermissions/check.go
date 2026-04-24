// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package pingicmppermissions

import (
	"errors"
	"syscall"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// Check attempts to create a raw ICMP socket to verify the agent has the required NET_RAW capability.
// Returns an IssueReport if the socket creation fails due to a permission error, nil otherwise.
func Check() (*healthplatform.IssueReport, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			return &healthplatform.IssueReport{
				IssueId: IssueID,
				Context: map[string]string{
					"error": err.Error(),
				},
				Tags: []string{"ping", "icmp", "permissions", "net_raw"},
			}, nil
		}
		// Unexpected error — do not report as a known issue
		return nil, err
	}

	// Socket created successfully — close it and report no issue
	_ = syscall.Close(fd)
	return nil, nil
}
