// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sbom TODO comment
package sbom

import (
	"time"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// This const block should have a comment or be unexported
const (
	ScanFilesystemType = "filesystem"
	ScanDaemonType     = "daemon"
)

// Report exported type should have comment or be unexported
type Report interface {
	ToCycloneDX() (*cyclonedxgo.BOM, error)
}

// ScanOptions exported type should have comment or be unexported
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

// ScanRequest exported type should have comment or be unexported
type ScanRequest interface {
	Collector() string
	Type() string
	ID() string
}

// ScanResult exported type should have comment or be unexported
type ScanResult struct {
	Error     error
	Report    Report
	CreatedAt time.Time
	Duration  time.Duration
	ImgMeta   *workloadmeta.ContainerImageMetadata
}
