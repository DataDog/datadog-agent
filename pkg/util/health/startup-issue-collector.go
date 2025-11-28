// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package health provides utilities for collecting and reporting agent health issues
package health

import (
	"sync"
)

// StartupIssue represents an issue detected during agent startup before components are initialized
type StartupIssue struct {
	IssueID string            // Issue ID for registry lookup (e.g., "docker-socket-permission-denied")
	Context map[string]string // Context data for the issue
}

var (
	startupIssues []StartupIssue
	mutex         sync.RWMutex
)

// CollectStartupIssue stores an issue detected during agent startup
func CollectStartupIssue(issueID string, context map[string]string) {
	mutex.Lock()
	defer mutex.Unlock()

	startupIssues = append(startupIssues, StartupIssue{
		IssueID: issueID,
		Context: context,
	})
}

// GetAndClearStartupIssues returns all collected startup issues and clears the storage
func GetAndClearStartupIssues() []StartupIssue {
	mutex.Lock()
	defer mutex.Unlock()

	collected := make([]StartupIssue, len(startupIssues))
	copy(collected, startupIssues)

	// Clear the storage
	startupIssues = nil

	return collected
}
