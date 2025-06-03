// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package hostmap provides constants for the hostmap.
package hostmap

import (
	conventions "go.opentelemetry.io/otel/semconv/v1.18.0"
)

// Platform related OpenTelemetry Semantic Conventions for resource attributes.
// These are NOT in the specification and will be submitted for approval.
const (
	attributeKernelName    = "os.kernel.name"
	attributeKernelRelease = "os.kernel.release"
	attributeKernelVersion = "os.kernel.version"
)

// This set of constants represent fields in the Gohai payload's Platform field.
const (
	fieldPlatformHostname         = "hostname"
	fieldPlatformOS               = "os"
	fieldPlatformGOOS             = "GOOS"
	fieldPlatformGOOARCH          = "GOOARCH"
	fieldPlatformProcessor        = "processor"
	fieldPlatformMachine          = "machine"
	fieldPlatformHardwarePlatform = "hardware_platform"
	fieldPlatformKernelName       = "kernel_name"
	fieldPlatformKernelRelease    = "kernel_release"
	fieldPlatformKernelVersion    = "kernel_version"
)

// platformAttributesMap defines the mapping between Gohai fieldPlatform fields
// and resource attribute names (semantic conventions or not).
var platformAttributesMap = map[string]string{
	fieldPlatformOS:               string(conventions.OSDescriptionKey),
	fieldPlatformGOOS:             string(conventions.OSTypeKey),
	fieldPlatformGOOARCH:          string(conventions.HostArchKey),
	fieldPlatformProcessor:        string(conventions.HostArchKey),
	fieldPlatformMachine:          string(conventions.HostArchKey),
	fieldPlatformHardwarePlatform: string(conventions.HostArchKey),
	fieldPlatformKernelName:       attributeKernelName,
	fieldPlatformKernelRelease:    attributeKernelRelease,
	fieldPlatformKernelVersion:    attributeKernelVersion,
}

// CPU related OpenTelemetry Semantic Conventions for resource attributes.
// TODO: Replace by conventions constants once available.
const (
	attributeHostCPUVendorID    = "host.cpu.vendor.id"
	attributeHostCPUModelName   = "host.cpu.model.name"
	attributeHostCPUFamily      = "host.cpu.family"
	attributeHostCPUModelID     = "host.cpu.model.id"
	attributeHostCPUStepping    = "host.cpu.stepping"
	attributeHostCPUCacheL2Size = "host.cpu.cache.l2.size"
)

// CPU related OpenTelemetry Semantic Conventions for metrics.
// TODO: Replace by conventions constants once available.
const (
	metricSystemCPUPhysicalCount = "system.cpu.physical.count"
	metricSystemCPULogicalCount  = "system.cpu.logical.count"
	metricSystemCPUFrequency     = "system.cpu.frequency"
	metricSystemMemoryLimit      = "system.memory.limit"
)

// This set of constants represent fields in the Gohai payload's CPU field.
const (
	fieldCPUVendorID          = "vendor_id"
	fieldCPUModelName         = "model_name"
	fieldCPUCacheSize         = "cache_size"
	fieldCPUFamily            = "family"
	fieldCPUModel             = "model"
	fieldCPUStepping          = "stepping"
	fieldCPUCores             = "cpu_cores"
	fieldCPULogicalProcessors = "cpu_logical_processors"
	fieldCPUMHz               = "mhz"
)

// cpuAttributesMap defines the mapping between Gohai fieldCPU fields
// and resource attribute names (semantic conventions or not).
var cpuAttributesMap = map[string]string{
	fieldCPUVendorID:  attributeHostCPUVendorID,
	fieldCPUModelName: attributeHostCPUModelName,
	fieldCPUCacheSize: attributeHostCPUCacheL2Size,
	fieldCPUFamily:    attributeHostCPUFamily,
	fieldCPUModel:     attributeHostCPUModelID,
	fieldCPUStepping:  attributeHostCPUStepping,
}

type cpuMetricsData struct {
	FieldName        string
	ConversionFactor float64
}

var cpuMetricsMap = map[string]cpuMetricsData{
	metricSystemCPUPhysicalCount: {FieldName: fieldCPUCores},
	metricSystemCPULogicalCount:  {FieldName: fieldCPULogicalProcessors},
	metricSystemCPUFrequency:     {FieldName: fieldCPUMHz, ConversionFactor: 1e-6},
}

// TrackedMetrics is the set of metrics that are tracked by the hostmap.
var TrackedMetrics = map[string]struct{}{
	metricSystemCPUPhysicalCount: {},
	metricSystemCPULogicalCount:  {},
	metricSystemCPUFrequency:     {},
	metricSystemMemoryLimit:      {},
}

// Network related OpenTelemetry Semantic Conventions for resource attributes.
// TODO: Replace by conventions constants once available.
const (
	attributeHostIP  = "host.ip"
	attributeHostMAC = "host.mac"
)

// This set of constants represent fields in the Gohai payload's Network field.
const (
	fieldNetworkIPAddressIPv4 = "ipaddress"
	fieldNetworkIPAddressIPv6 = "ipaddressv6"
	fieldNetworkMACAddress    = "macaddress"
)
