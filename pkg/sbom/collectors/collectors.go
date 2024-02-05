// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectors holds collectors related files
package collectors

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
)

// ScanType defines the scan type of the collector
type ScanType string

const (
	// ContainerImageScanType defines the container image scan type
	ContainerImageScanType ScanType = "container-image"
	// HostScanType defines the host scan type
	HostScanType ScanType = "host"
)

// Collector interface
type Collector interface {
	// Type returns the scan type of the collector
	Type() ScanType
	// CleanCache cleans the collector cache
	CleanCache() error
	// Init initializes the collector
	Init(config.Config) error
	// Scan performs a scan
	Scan(context.Context, sbom.ScanRequest, sbom.ScanOptions) sbom.ScanResult
}

// Collectors values
var Collectors map[string]Collector

// RegisterCollector registers given collector
func RegisterCollector(name string, collector Collector) {
	Collectors[name] = collector
}

func init() {
	Collectors = make(map[string]Collector)
}
