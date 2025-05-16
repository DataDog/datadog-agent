// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package module

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	heartbeatParam = "heartbeat"
	heartbeatTime  = 15 * time.Minute

	pidsParam = "pids"
)

type params struct {
	heartbeatTime time.Duration
	pids          []int
}

func defaultParams() params {
	return params{
		heartbeatTime: heartbeatTime,
	}
}

func (params params) updateQuery(query url.Values) {
	query.Set(heartbeatParam, strconv.Itoa(int(params.heartbeatTime.Seconds())))

	if len(params.pids) > 0 {
		pidsStr := make([]string, len(params.pids))
		for i, pid := range params.pids {
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
		return heartbeatTime, nil
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

func parseParams(query url.Values) (params, error) {
	params := defaultParams()

	heartbeat, err := parseHeartbeat(query)
	if err != nil {
		return params, err
	}
	params.heartbeatTime = heartbeat

	pids, err := parsePids(query)
	if err != nil {
		return params, err
	}
	params.pids = pids

	return params, nil
}
