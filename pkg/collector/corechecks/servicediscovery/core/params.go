// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package core provides the core functionality for service discovery.
package core

import (
	"net/url"
	"strconv"
	"time"
)

const (
	heartbeatParam = "heartbeat"
	// HeartbeatTime defines the interval for heartbeat updates.
	HeartbeatTime = 15 * time.Minute
)

// Params represents the parameters for service discovery requests.
type Params struct {
	HeartbeatTime time.Duration
}

// DefaultParams returns a new Params instance with default values.
func DefaultParams() Params {
	return Params{
		HeartbeatTime: HeartbeatTime,
	}
}

// UpdateQuery updates the URL query parameters with the current Params values.
func (params Params) UpdateQuery(query url.Values) {
	query.Set(heartbeatParam, strconv.Itoa(int(params.HeartbeatTime.Seconds())))
}

func parseDuration(raw string) (time.Duration, error) {
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	return time.Duration(val) * time.Second, err
}

// ParseParams parses URL query parameters into a Params struct.
func ParseParams(query url.Values) (Params, error) {
	params := DefaultParams()

	raw := query.Get(heartbeatParam)
	if raw != "" {
		heartbeat, err := parseDuration(raw)
		if err != nil {
			return params, err
		}
		params.HeartbeatTime = heartbeat
	}

	return params, nil
}
