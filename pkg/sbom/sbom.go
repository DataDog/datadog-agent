// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sbom holds sbom related files
package sbom

import (
	"time"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	ScanFilesystemType = "filesystem" // ScanFilesystemType defines the type for file-system scan
	ScanDaemonType     = "daemon"     // ScanDaemonType defines the type for daemon scan
)

// Report defines the report interface
type Report interface {
	ToCycloneDX() (*cyclonedxgo.BOM, error)
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
	NoCache          bool // Caching doesn't really provide any value when scanning filesystem as the filesystem has to be walked to compute the keys
}

// ScanOptionsFromConfig loads the scanning options from the configuration
func ScanOptionsFromConfig(cfg config.Config, containers bool) (scanOpts ScanOptions) {
	if containers {
		scanOpts.CheckDiskUsage = config.Datadog.GetBool("sbom.container_image.check_disk_usage")
		scanOpts.MinAvailableDisk = uint64(config.Datadog.GetSizeInBytes("sbom.container_image.min_available_disk"))
		scanOpts.Timeout = time.Duration(config.Datadog.GetInt("sbom.container_image.scan_timeout")) * time.Second
		scanOpts.WaitAfter = time.Duration(config.Datadog.GetInt("sbom.container_image.scan_interval")) * time.Second
		scanOpts.Analyzers = config.Datadog.GetStringSlice("sbom.container_image.analyzers")
	}

	if len(scanOpts.Analyzers) == 0 {
		scanOpts.Analyzers = config.Datadog.GetStringSlice("sbom.host.analyzers")
	}

	return
}

// ScanRequest defines the scan request interface
type ScanRequest interface {
	Collector() string
	Type() string
	ID() string
}

// ScanResult defines the scan result
type ScanResult struct {
	Error     error
	Report    Report
	CreatedAt time.Time
	Duration  time.Duration
	ImgMeta   *workloadmeta.ContainerImageMetadata
}
