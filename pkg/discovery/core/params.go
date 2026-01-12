// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package core provides the core functionality for service discovery.
package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// HeartbeatTime defines the interval for heartbeat updates.
	HeartbeatTime = 15 * time.Minute
)

// Params represents the parameters for service discovery requests.
type Params struct {
	NewPids       []int32 `json:"new_pids,omitempty"`       // PIDs never seen before, require full service info
	HeartbeatPids []int32 `json:"heartbeat_pids,omitempty"` // PIDs needing heartbeat refresh, minimal updates
}

// ToJSON serializes the Params to JSON bytes.
func (params Params) ToJSON() ([]byte, error) {
	return json.Marshal(params)
}

// FromJSON deserializes JSON bytes into a Params struct.
func FromJSON(data []byte) (Params, error) {
	params := Params{}
	if err := json.Unmarshal(data, &params); err != nil {
		return params, fmt.Errorf("failed to unmarshal params: %w", err)
	}
	return params, nil
}

// ParseParamsFromRequest parses parameters from JSON body.
func ParseParamsFromRequest(req *http.Request) (Params, error) {
	params := Params{}

	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return params, fmt.Errorf("failed to read request body: %w", err)
		}

		if len(body) > 0 {
			params, err = FromJSON(body)
			if err != nil {
				return params, fmt.Errorf("failed to parse JSON body: %w", err)
			}
		}
	}

	return params, nil
}
