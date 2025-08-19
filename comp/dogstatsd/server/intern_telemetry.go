// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import "github.com/DataDog/datadog-agent/comp/core/telemetry"

type stringInternerTelemetry struct {
	enabled bool

	resets               telemetry.Counter
	size                 telemetry.Gauge
	bytes                telemetry.Gauge
	hits                 telemetry.Counter
	miss                 telemetry.Counter
	globaltlmSIRStrBytes telemetry.SimpleHistogram
}

type stringInternerInstanceTelemetry struct {
	enabled  bool
	curBytes int

	resets               telemetry.SimpleCounter
	size                 telemetry.SimpleGauge
	bytes                telemetry.SimpleGauge
	hits                 telemetry.SimpleCounter
	miss                 telemetry.SimpleCounter
	globaltlmSIRStrBytes telemetry.SimpleHistogram
}

func newSiTelemetry(enabled bool, telemetry telemetry.Component) *stringInternerTelemetry {
	if enabled {
		return &stringInternerTelemetry{
			enabled: enabled,
			globaltlmSIRStrBytes: telemetry.NewSimpleHistogram("dogstatsd", "string_interner_str_bytes",
				"Number of times string with specific length were added",
				[]float64{1, 2, 4, 8, 16, 32, 64, 128}),
			resets: telemetry.NewCounter("dogstatsd", "string_interner_resets", []string{"interner_id"}, "Amount of resets of the string interner used in dogstatsd"),
			size:   telemetry.NewGauge("dogstatsd", "string_interner_entries", []string{"interner_id"}, "Number of entries in the string interner"),
			bytes:  telemetry.NewGauge("dogstatsd", "string_interner_bytes", []string{"interner_id"}, "Number of bytes stored in the string interner"),
			hits:   telemetry.NewCounter("dogstatsd", "string_interner_hits", []string{"interner_id"}, "Number of times string interner returned an existing string"),
			miss:   telemetry.NewCounter("dogstatsd", "string_interner_miss", []string{"interner_id"}, "Number of times string interner created a new string object"),
		}
	}

	return &stringInternerTelemetry{
		enabled: enabled,
	}
}

// PrepareForId creates an instance of stringInternerInstanceTelemetry for a specific id.
func (s *stringInternerTelemetry) PrepareForID(id string) *stringInternerInstanceTelemetry {
	if s.enabled {
		return &stringInternerInstanceTelemetry{
			enabled:              true,
			resets:               s.resets.WithValues(id),
			size:                 s.size.WithValues(id),
			bytes:                s.bytes.WithValues(id),
			hits:                 s.hits.WithValues(id),
			miss:                 s.miss.WithValues(id),
			globaltlmSIRStrBytes: s.globaltlmSIRStrBytes,
		}
	}

	return &stringInternerInstanceTelemetry{
		enabled: false,
	}
}

// Hit increments the hit counter.
func (si *stringInternerInstanceTelemetry) Hit() {
	if si.enabled {
		si.hits.Inc()
	}
}

// Reset increments the reset counter and updates the size and bytes gauges.
func (si *stringInternerInstanceTelemetry) Reset(length int) {
	if si.enabled {
		si.resets.Inc()
		si.bytes.Sub(float64(si.curBytes))
		si.size.Sub(float64(length))
		si.curBytes = 0
	}
}

// Miss increments the miss counter and updates the size and bytes gauges.
func (si *stringInternerInstanceTelemetry) Miss(length int) {
	if si.enabled {
		si.miss.Inc()
		si.size.Inc()
		si.bytes.Add(float64(length))
		si.globaltlmSIRStrBytes.Observe(float64(length))
		si.curBytes += length
	}
}
