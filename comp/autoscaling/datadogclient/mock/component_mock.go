// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package datadogclient

import (
	"gopkg.in/zorkian/go-datadog-api.v2"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
)

// MockComponent implements mock-specific methods.
type MockComponent interface {
	datadogclient.Component
	SetQueryMetricsFunc(queryMetricsFunc func(from, to int64, query string) ([]datadog.Series, error))
	SetGetRateLimitsFunc(getRateLimitsFunc func() map[string]datadog.RateLimit)
}
