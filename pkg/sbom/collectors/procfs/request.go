// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procfs

import (
	"github.com/DataDog/datadog-agent/pkg/sbom/types"
)

// scanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type scanRequest struct {
	ContainerID string
}

// NewScanRequest creates a new scan request
func NewScanRequest(containerID string) types.ScanRequest {
	return scanRequest{ContainerID: containerID}
}

// Collector returns the collector name
func (r scanRequest) Collector() string {
	return "procfs"
}

// Type returns the scan request type
func (r scanRequest) Type(types.ScanOptions) string {
	return types.ScanFilesystemType
}

// ID returns the scan request ID
func (r scanRequest) ID() string {
	return r.ContainerID
}
