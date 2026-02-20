// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

// IsUDSAvailable always returns false on Windows. DogStatsD does not use
// Unix domain datagram sockets on Windows.
func IsUDSAvailable(_ string) bool {
	return false
}
