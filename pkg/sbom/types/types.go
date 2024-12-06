// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types holds sbom related types
package types

import "time"

// ScanRequest defines the scan request interface
type ScanRequest interface {
	Collector() string
	Type(ScanOptions) string
	ID() string
}

// ScanOptions defines the scan options
type ScanOptions struct {
	Analyzers        []string
	CheckDiskUsage   bool
	MinAvailableDisk uint64
	Timeout          time.Duration
	WaitAfter        time.Duration
	Fast             bool
	CollectFiles     bool
	UseMount         bool
	OverlayFsScan    bool
}

const (
	ScanFilesystemType = "filesystem" // ScanFilesystemType defines the type for file-system scan
	ScanDaemonType     = "daemon"     // ScanDaemonType defines the type for daemon scan
)
