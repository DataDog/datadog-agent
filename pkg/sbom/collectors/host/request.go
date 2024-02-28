// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy || windows

package host

import (
	"io/fs"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
)

const (
	collectorName = "host"
)

// ScanRequest defines a scan request
type ScanRequest struct {
	Path string
	FS   fs.FS
}

// Collector returns the collector name
func (r *ScanRequest) Collector() string {
	return collectorName
}

// Type returns the scan request type
func (r *ScanRequest) Type() string {
	return sbom.ScanFilesystemType
}

// ID returns the scan request ID
func (r *ScanRequest) ID() string {
	return r.Path
}

func init() {
	collectors.RegisterCollector(collectorName, &Collector{})
}
