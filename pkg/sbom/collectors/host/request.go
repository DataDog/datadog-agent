// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"io/fs"

	"github.com/DataDog/datadog-agent/pkg/sbom/types"
)

// scanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type scanRequest struct {
	Path string
	FS   fs.FS
}

// NewScanRequest creates a new scan request
func NewScanRequest(path string, fs fs.FS) types.ScanRequest {
	return scanRequest{Path: path, FS: fs}
}

// Collector returns the collector name
func (r scanRequest) Collector() string {
	return "host"
}

// Type returns the scan request type
func (r scanRequest) Type(types.ScanOptions) string {
	return "filesystem"
}

// ID returns the scan request ID
func (r scanRequest) ID() string {
	return r.Path
}
