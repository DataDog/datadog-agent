// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy || (windows && wmi)

package host

import (
	"io/fs"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
)

// channelSize defines the result channel size
// It doesn't need more than 1 because the host collector should
// not trigger multiple scans at the same time unlike for container-images.
const channelSize = 1

// scanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type scanRequest struct {
	Path string
	FS   fs.FS
}

// NewScanRequest creates a new scan request
func NewScanRequest(path string, fs fs.FS) sbom.ScanRequest {
	return scanRequest{Path: path, FS: fs}
}

// Collector returns the collector name
func (r scanRequest) Collector() string {
	return collectors.HostCollector
}

// Type returns the scan request type
func (r scanRequest) Type(sbom.ScanOptions) string {
	return sbom.ScanFilesystemType
}

// ID returns the scan request ID
func (r scanRequest) ID() string {
	return r.Path
}
