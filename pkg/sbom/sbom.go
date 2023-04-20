// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"time"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/config"
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
	Fast             bool
}

// ScanOptionsFromConfig loads the scanning options from the configuration
func ScanOptionsFromConfig(cfg config.Config, containers bool) (scanOpts ScanOptions) {
	if containers {
		scanOpts.CheckDiskUsage = config.Datadog.GetBool("container_image_collection.sbom.check_disk_usage")
		scanOpts.MinAvailableDisk = uint64(config.Datadog.GetSizeInBytes("container_image_collection.sbom.min_available_disk"))
		scanOpts.Timeout = time.Duration(config.Datadog.GetInt("container_image_collection.sbom.scan_timeout")) * time.Second
		scanOpts.WaitAfter = time.Duration(config.Datadog.GetInt("container_image_collection.sbom.scan_interval")) * time.Second
		scanOpts.Analyzers = config.Datadog.GetStringSlice("container_image_collection.sbom.analyzers")
	}

	if len(scanOpts.Analyzers) == 0 {
		scanOpts.Analyzers = config.Datadog.GetStringSlice("sbom.analyzers")
	}

	return
}

type ScanRequest interface {
	Collector() string
	Type() string
	ID() string
}

type ScanResult struct {
	Error     error
	Report    Report
	CreatedAt time.Time
	Duration  time.Duration
}
