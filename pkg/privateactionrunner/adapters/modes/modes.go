// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package modes

// Mode is a runner operation mode (e.g., pull).
type Mode string

const (
	// ModePull represents the pull-based execution mode
	ModePull Mode = "pull"
)

// ToStrings converts modes to strings.
func ToStrings(m []Mode) []string {
	var res []string
	for _, mode := range m {
		res = append(res, string(mode))
	}
	return res
}

// MetricTag returns the mode as a metric tag value.
func (m Mode) MetricTag() string {
	return string(m)
}
