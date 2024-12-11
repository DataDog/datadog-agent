// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package datadogclient provides a client to query the datadog API
package datadogclient

import (
	"gopkg.in/zorkian/go-datadog-api.v2"
)

// team: container-integrations

// Component is the component type.
type Component interface {
	// QueryMetrics takes as input from, to (seconds from Unix Epoch) and query string and then requests
	// timeseries data for that time peried
	QueryMetrics(from, to int64, query string) ([]datadog.Series, error)

	// GetRateLimitStats is a threadsafe getter to retrieve the rate limiting stats associated with the Client.
	GetRateLimitStats() map[string]datadog.RateLimit
}
