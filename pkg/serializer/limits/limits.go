// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package limits provides payload size limits for the serializer.
// These limits match the intake server's enforced limits.
package limits

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
)

// Endpoint identifies payload types with different size limits.
type Endpoint int

const (
	// Default covers most endpoints (events, sketches, service_checks).
	Default Endpoint = iota
	// Series has specific limits for metrics series payloads.
	Series
	// Metadata has fixed limits enforced by the intake server (not configurable).
	Metadata
)

// Limits contains the size constraints for a payload type.
type Limits struct {
	// MaxCompressed is the maximum compressed payload size in bytes.
	// Zero means no limit or not applicable.
	MaxCompressed int
	// MaxUncompressed is the maximum uncompressed payload size in bytes.
	MaxUncompressed int
	// MaxItems is the maximum number of items (e.g., points for series).
	// Zero means unlimited.
	MaxItems int
	// TargetBatch is the target size for batching, used by metadata.
	// Zero means not applicable.
	TargetBatch int
}

// Fixed limits for metadata payloads. These are enforced by the intake server
// and are not configurable. The intake rejects metadata payloads exceeding
// 1MB uncompressed with HTTP 413.
const (
	MetadataMaxUncompressed = 1024 * 1024 // 1MB
	MetadataTargetBatch     = 800 * 1024  // 800KB (safety margin for JSON overhead)
)

// Get returns the limits for the specified endpoint type.
// For configurable limits, values are read from the config component.
// For fixed limits (Metadata), hardcoded values are returned.
func Get(e Endpoint, cfg config.Component) Limits {
	switch e {
	case Series:
		return Limits{
			MaxCompressed:   cfg.GetInt("serializer_max_series_payload_size"),
			MaxUncompressed: cfg.GetInt("serializer_max_series_uncompressed_payload_size"),
			MaxItems:        cfg.GetInt("serializer_max_series_points_per_payload"),
		}
	case Metadata:
		return Limits{
			MaxUncompressed: MetadataMaxUncompressed,
			TargetBatch:     MetadataTargetBatch,
		}
	default:
		return Limits{
			MaxCompressed:   cfg.GetInt("serializer_max_payload_size"),
			MaxUncompressed: cfg.GetInt("serializer_max_uncompressed_payload_size"),
		}
	}
}

// Exceeds returns true if the given sizes exceed the limits.
func (l Limits) Exceeds(compressed, uncompressed int) bool {
	if l.MaxCompressed > 0 && compressed > l.MaxCompressed {
		return true
	}
	if l.MaxUncompressed > 0 && uncompressed > l.MaxUncompressed {
		return true
	}
	return false
}
