// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package utils provides utility functions for the private action runner.
package utils

const (
	// JwtHeaderName is the header name for JWT authentication.
	JwtHeaderName = "X-Datadog-OnPrem-JWT"
	// VersionHeaderName is the header name for version information.
	VersionHeaderName = "X-Datadog-OnPrem-Version"
	// ModeHeaderName is the header name for mode information.
	ModeHeaderName = "X-Datadog-OnPrem-Modes"
)
