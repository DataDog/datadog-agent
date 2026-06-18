// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package serverimpl

func defaultGPUJobMetadataProcessExists(_ uint32) bool {
	// PID cleanup is only meaningful for the Linux UDS SO_PASSCRED path. Keep
	// records alive elsewhere and rely on explicit end events or TTL fallback.
	return true
}
