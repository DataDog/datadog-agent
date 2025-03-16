// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package module

import (
	"net/url"
	"strconv"
	"time"
)

const (
	heartbeatParam = "heartbeat"
	heartbeatTime  = 15 * time.Minute
)

type params struct {
	heartbeatTime time.Duration
}

func defaultParams() params {
	return params{
		heartbeatTime: heartbeatTime,
	}
}

func (params params) updateQuery(query url.Values) {
	query.Set(heartbeatParam, strconv.Itoa(int(params.heartbeatTime.Seconds())))
}

func parseDuration(raw string) (time.Duration, error) {
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	return time.Duration(val) * time.Second, err
}

func parseParams(query url.Values) (params, error) {
	params := defaultParams()

	raw := query.Get(heartbeatParam)
	if raw != "" {
		heartbeat, err := parseDuration(raw)
		if err != nil {
			return params, err
		}
		params.heartbeatTime = heartbeat
	}

	return params, nil
}
