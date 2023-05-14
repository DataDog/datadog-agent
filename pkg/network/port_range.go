// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

const (
	ephemeralRangeStart = 32768
	ephemeralRangeEnd   = 60999
)

// IsEphemeralPort returns true if a port belongs to the ephemeral range
// This is mostly a placeholder for now as we have work planned for a
// platform-agnostic solution that will, among other things, source these values
// from procfs for Linux hosts
func IsEphemeralPort(port int) bool {
	return port >= ephemeralRangeStart && port <= ephemeralRangeEnd
}
