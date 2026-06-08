// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

// PRMRequest represents a single privileged PRM query to be executed by system-probe.
type PRMRequest struct {
	DeviceUUID string `json:"device_uuid"`
	Port       int    `json:"port"`
	Group      int    `json:"group"`
}

// PRMResponse contains the result of a PRMRequest.
type PRMResponse struct {
	Request  PRMRequest        `json:"request"`
	Counters map[string]uint64 `json:"counters,omitempty"`
	Error    string            `json:"error,omitempty"`
}
