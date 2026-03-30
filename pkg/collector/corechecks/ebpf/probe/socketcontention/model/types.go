// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

// SocketContentionStats contains stage-1 skeleton stats for the socket contention probe.
type SocketContentionStats struct {
	SocketInits uint64 `json:"socket_inits"`
}
