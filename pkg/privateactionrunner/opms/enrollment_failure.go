// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opms

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"
)

// EnrollmentFailure contains only bounded categories, never server detail/body.
// RetrySafe means this attempt is proven not to have created a runner.
type EnrollmentFailure struct {
	Category               string    `json:"category"`
	RetrySafe              bool      `json:"retry_safe"`
	ReconciliationRequired bool      `json:"reconciliation_required"`
	RetryAt                time.Time `json:"retry_at,omitempty"`
}

func (e *EnrollmentFailure) Error() string { return "enrollment: " + e.Category }

// RetryAfter preserves the server's lower bound. Unknown hints use the caller's
// fallback; timing is deliberately independent of permission to replay a POST.
func RetryAfter(value string, now time.Time, fallback time.Duration) time.Time {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
		const maxSeconds = int64((1<<63 - 1) / int64(time.Second))
		return now.Add(time.Duration(min(seconds, maxSeconds)) * time.Second)
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date
	}
	return now.Add(fallback)
}

func transportFailure(err error) *EnrollmentFailure {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return &EnrollmentFailure{Category: "not_submitted", RetrySafe: true}
	}
	return &EnrollmentFailure{Category: "transport_ambiguous", ReconciliationRequired: true}
}

func responseFailure(code int, body []byte, retryAfter string, now time.Time) *EnrollmentFailure {
	failure := &EnrollmentFailure{Category: "response_ambiguous", ReconciliationRequired: true}
	if retryAfter != "" {
		failure.RetryAt = RetryAfter(retryAfter, now, 5*time.Minute)
	}
	if code == 429 {
		// The public route has no uniquely identifiable no-creation 429
		// response contract in the inspected backend. Do not invent one.
		failure.Category = "throttled_unverified"
		failure.RetryAt = RetryAfter(retryAfter, now, 5*time.Minute)
		return failure
	}
	if code == 403 {
		failure.Category = "forbidden_unknown"
	}
	if code == 401 {
		failure.Category = "unauthorized_unverified"
	}
	if code == 400 {
		failure.Category = "invalid_request_unverified"
	}
	var document struct {
		Errors []struct {
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &document) != nil || len(document.Errors) != 1 {
		return failure
	}
	e := document.Errors[0]
	if e.Status != strconv.Itoa(code) {
		return failure
	}
	// Exact JSON:API error kinds verified BEFORE creation in OPMS / actions
	// server at dd-source 52e8d4f9929. These are API fields, not log matching.
	switch {
	case code == 403 && e.Title == "your organization has reached the maximum number of private action runners":
		return &EnrollmentFailure{Category: "quota", RetrySafe: true, RetryAt: failure.RetryAt}
	case code == 403 && e.Title == "required scope missing":
		return &EnrollmentFailure{Category: "missing_scope", RetrySafe: true, RetryAt: failure.RetryAt}
	case code == 401 && e.Title == "invalid auth context":
		return &EnrollmentFailure{Category: "invalid_credentials", RetrySafe: true, RetryAt: failure.RetryAt}
	case code == 400 && (e.Title == "error decoding request" || e.Title == "missing required field" || e.Title == "value not allowed"):
		return &EnrollmentFailure{Category: "invalid_request", RetryAt: failure.RetryAt}
	case code == 400 && e.Title == "a runner with this name already exists":
		return &EnrollmentFailure{Category: "name_conflict", ReconciliationRequired: true, RetryAt: failure.RetryAt}
	default:
		return failure
	}
}
