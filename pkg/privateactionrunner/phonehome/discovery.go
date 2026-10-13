// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	pollInterval = time.Minute
	slowRetry    = 15 * time.Minute
)

type discovery struct {
	eligible  bool
	reason    string
	delay     time.Duration
	transient bool
}

func validate(ctx context.Context, client *http.Client, endpoint, apiKey string) discovery {
	if apiKey == "" {
		return discovery{reason: "invalid_credentials", delay: slowRetry}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/v2/validate", nil)
	if err != nil {
		return discovery{reason: "invalid_endpoint", delay: slowRetry}
	}
	req.Header.Set("DD-API-KEY", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		// Neither transport errors (which can contain proxy credentials) nor
		// response bodies belong in status/logs.
		return discovery{reason: "discovery_unavailable", transient: true}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return discovery{reason: "discovery_rate_limited", delay: max(pollInterval, retryAfter(resp.Header.Get("Retry-After"), time.Now()))}
	case resp.StatusCode == http.StatusUnauthorized:
		return discovery{reason: "invalid_credentials", delay: slowRetry}
	case resp.StatusCode >= 500:
		return discovery{reason: "discovery_unavailable", transient: true}
	case resp.StatusCode != http.StatusOK:
		return discovery{reason: "discovery_http_" + strconv.Itoa(resp.StatusCode), delay: slowRetry}
	}
	var result struct {
		Data struct {
			Attributes struct {
				Valid  *bool    `json:"valid"`
				Scopes []string `json:"api_key_scopes"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || result.Data.Attributes.Valid == nil {
		return discovery{reason: "invalid_discovery_response", transient: true}
	}
	if !*result.Data.Attributes.Valid {
		return discovery{reason: "invalid_credentials", delay: slowRetry}
	}
	for _, scope := range result.Data.Attributes.Scopes {
		if scope == "private_action_runner_enroll" {
			return discovery{eligible: true, reason: "scope_present", delay: pollInterval}
		}
	}
	return discovery{reason: "missing_enrollment_scope", delay: pollInterval}
}

func retryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
		// Avoid duration overflow without retrying earlier than a huge hint.
		const maxSeconds = int64((1<<63 - 1) / int64(time.Second))
		return time.Duration(min(seconds, maxSeconds)) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return pollInterval
}
