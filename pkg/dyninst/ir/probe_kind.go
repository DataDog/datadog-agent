// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

//go:generate go run golang.org/x/tools/cmd/stringer -type=ProbeKind -linecomment -output probe_kind_string.go

// ProbeKind is the kind of probe.
type ProbeKind uint8

const (
	_ ProbeKind = iota

	// ProbeKindLog is a probe that emits a log.
	ProbeKindLog
	// ProbeKindSpan is a probe that emits a span.
	ProbeKindSpan
	// ProbeKindMetric is a probe that updates a metric.
	ProbeKindMetric
	// ProbeKindSnapshot is a probe that emits a snapshot.
	//
	// Internally in rcjson these are log probes with capture_snapshot set to
	// true.
	ProbeKindSnapshot

	maxProbeKind uint8 = iota
)

// IsValid returns true if the probe kind is valid.
func (k ProbeKind) IsValid() bool {
	return k > 0 && uint8(k) < maxProbeKind
}
