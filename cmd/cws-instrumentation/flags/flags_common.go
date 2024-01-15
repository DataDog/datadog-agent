// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flags holds default flags for the CWS injector
package flags

const (
	// CWSVolumeMount is used to provide the path to the CWS Volume mount
	CWSVolumeMount = "cws-volume-mount"

	// Data represent the user session data to inject
	Data = "data"
	// SessionType represents the type of the user session that is being injected
	SessionType = "session-type"
)
