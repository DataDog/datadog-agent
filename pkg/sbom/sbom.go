// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"time"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
)

const (
	ScanFilesystemType = "filesystem"
	ScanDaemonType     = "daemon"
)

type Report interface {
	ToCycloneDX() (*cyclonedxgo.BOM, error)
}

type ScanOptions struct {
	Analyzers        []string
	CheckDiskUsage   bool
	MinAvailableDisk uint64
	Timeout          time.Duration
	WaitAfter        time.Duration
}

type ScanRequest interface {
	Collector() string
	Type() string
	ID() string
}

type ScanResult struct {
	Report    Report
	CreatedAt time.Time
	Duration  time.Duration
}
