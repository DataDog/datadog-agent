// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package datadogclientimpl provides the noop datadogclientimpl component
package datadogclientimpl

import (
	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

// DatadogclientImplNone is a noop datadogclient implementation
type DatadogclientImplNone struct{}

var _ datadogclient.Component = (*DatadogclientImplNone)(nil) // ensure: DatadogclientImplNone implements the interface

// NewNone creates a new noop datadogclient component
func NewNone() datadogclient.Component {
	return &DatadogclientImplNone{}
}

// QueryMetrics does nothing for the noop datadogclient implementation
func (d *DatadogclientImplNone) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	// noop
	return nil, nil
}

// GetRateLimitStats does nothing for the noop datadogclient implementation
func (d *DatadogclientImplNone) GetRateLimitStats() map[string]datadog.RateLimit {
	// noop
	return nil
}
