// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package core provides the core functionality for service discovery.
package core

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	heartbeatParam = "heartbeat"
	// HeartbeatTime defines the interval for heartbeat updates.
	HeartbeatTime = 15 * time.Minute

	pidsParam = "pids"
)

// Params represents the parameters for service discovery requests.
type Params struct {
	HeartbeatTime time.Duration
	Pids          []int
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

	if len(params.Pids) > 0 {
		pidsStr := make([]string, len(params.Pids))
		for i, pid := range params.Pids {
			pidsStr[i] = strconv.Itoa(pid)
		}
		query.Set(pidsParam, strings.Join(pidsStr, ","))
	}
}

func parseDuration(raw string) (time.Duration, error) {
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	return time.Duration(val) * time.Second, err
}

func parseHeartbeat(query url.Values) (time.Duration, error) {
	raw := query.Get(heartbeatParam)
	if raw == "" {
		return HeartbeatTime, nil
	}

	heartbeat, err := parseDuration(raw)
	if err != nil {
		return 0, err
	}

	return heartbeat, nil
}

func parsePids(query url.Values) ([]int, error) {
	if !query.Has(pidsParam) {
		return nil, nil
	}

	pidsRaw := query.Get(pidsParam)

	pidsStr := strings.Split(pidsRaw, ",")
	pids := make([]int, 0, len(pidsStr))
	for _, raw := range pidsStr {
		pid, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pid %q: %w", raw, err)
		}

		pids = append(pids, pid)
	}

	return pids, nil
}

// ParseParams parses URL query parameters into a Params struct.
func ParseParams(query url.Values) (Params, error) {
	params := DefaultParams()

	heartbeat, err := parseHeartbeat(query)
	if err != nil {
		return params, err
	}
	params.HeartbeatTime = heartbeat

	pids, err := parsePids(query)
	if err != nil {
		return params, err
	}
	params.Pids = pids

	return params, nil
}
