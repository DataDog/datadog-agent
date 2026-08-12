// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package statusapi

const namedPipePath = `\\.\pipe\DD_INSTALLER_STATUS`

// Endpoint returns the named pipe the status API listens on. Both sides derive it
// from here so they cannot drift apart.
func Endpoint() string {
	return namedPipePath
}
