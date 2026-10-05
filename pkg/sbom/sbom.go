// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sbom holds sbom related files
package sbom

import (
	"errors"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ScanFilesystemType = "filesystem"  // ScanFilesystemType defines the type for file-system scan
	ScanDaemonType     = "daemon"      // ScanDaemonType defines the type for daemon scan
	ScanMethodTagName  = "scan_method" // ScanMethodTagName defines the tag name for scan method
)

// Names of the CycloneDX component properties added by the runtime enrichment
// of a SBOM, describing how the package was seen being used at runtime. They
// are declared here so that the producer (the runtime security SBOM resolver),
// the merger (the remote SBOM collector) and the consumers agree on them.
const (
	LastAccessProperty    = "LastSeenRunning" // LastAccessProperty holds the last time the package was seen running, as a unix timestamp
	HasSetSuidBitProperty = "HasSetSuidBit"   // HasSetSuidBitProperty reports whether one of the package files has the setuid bit set
	RunningAsRootProperty = "RunningAsRoot"   // RunningAsRootProperty reports whether the package was seen running as root
)

// IsEnriched reports whether bom went through runtime enrichment, i.e. whether
// any of its components carries the runtime properties above. An enriched BOM
// carries them on its very first component, so the scan only runs to completion
// for a BOM that was never enriched.
func IsEnriched(bom *cyclonedx_v1_4.Bom) bool {
	if bom == nil {
		return false
	}

	for _, component := range bom.Components {
		if component == nil {
			continue
		}

		for _, property := range component.Properties {
			if property != nil && property.Name == LastAccessProperty {
				return true
			}
		}
	}

	return false
}

// ErrScanNotSupported reports that a scan can never succeed for the given image
// and must not be retried. Exporting an image on a remote snapshotter such as
// nydus to a tarball is such a case. Its layers live in the snapshotter rather
// than the content store, so the export cannot read them however often the scan
// is retried.
var ErrScanNotSupported = errors.New("sbom scan not supported for this image")

// Report defines the report interface
type Report interface {
	ToCycloneDX() *cyclonedx_v1_4.Bom
	ID() string
}

// ScanOptionsFromConfigForContainers loads the scanning options from the configuration
func ScanOptionsFromConfigForContainers(cfg config.Component) ScanOptions {
	return ScanOptions{
		CheckDiskUsage:   cfg.GetBool("sbom.container_image.check_disk_usage"),
		MinAvailableDisk: uint64(cfg.GetSizeInBytes("sbom.container_image.min_available_disk")),
		Timeout:          time.Duration(cfg.GetInt("sbom.container_image.scan_timeout")) * time.Second,
		WaitAfter:        time.Duration(cfg.GetInt("sbom.container_image.scan_interval")) * time.Second,
		Analyzers:        cfg.GetStringSlice("sbom.container_image.analyzers"),
		UseMount:         cfg.GetBool("sbom.container_image.use_mount"),
		OverlayFsScan:    cfg.GetBool("sbom.container_image.overlayfs_direct_scan"),
		AdditionalDirs:   cfg.GetStringSlice("sbom.container_image.additional_directories"),
	}
}

// ScanOptionsFromConfigForHosts loads the scanning options from the configuration
func ScanOptionsFromConfigForHosts(cfg config.Component) ScanOptions {
	return ScanOptions{
		Analyzers:      cfg.GetStringSlice("sbom.host.analyzers"),
		AdditionalDirs: cfg.GetStringSlice("sbom.host.additional_directories"),
	}
}

// ScanRequest defines the scan request interface
type ScanRequest = types.ScanRequest

// ScanOptions defines the scan options
type ScanOptions = types.ScanOptions

// ScanResult defines the scan result
type ScanResult struct {
	Error            error
	Report           Report
	CreatedAt        time.Time
	Duration         time.Duration
	GenerationMethod string
	ImgMeta          *workloadmeta.ContainerImageMetadata
	RequestID        string
}

// ConvertScanResultToSBOM converts an SBOM scan result to a workloadmeta SBOM.
func (result *ScanResult) ConvertScanResultToSBOM() *workloadmeta.SBOM {
	status := workloadmeta.Success
	reportedError := ""
	var report *cyclonedx_v1_4.Bom

	if result.Error != nil {
		log.Errorf("SBOM generation failed for image: %v", result.Error)
		status = workloadmeta.Failed
		reportedError = result.Error.Error()
	} else {
		report = result.Report.ToCycloneDX()
	}

	sbom := &workloadmeta.SBOM{
		CycloneDXBOM:       report,
		GenerationTime:     result.CreatedAt,
		GenerationDuration: result.Duration,
		GenerationMethod:   result.GenerationMethod,
		Status:             status,
		Error:              reportedError,
	}

	return sbom
}
