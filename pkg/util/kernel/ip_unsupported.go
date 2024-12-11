// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package kernel

// IsIPv6Enabled returns whether or not IPv6 has been enabled on the host
func IsIPv6Enabled() bool {
	return true
}
