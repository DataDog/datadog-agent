// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package datadogclientimpl provides the noop datadogclientimpl component
package datadogclientimpl

import (
	"gopkg.in/zorkian/go-datadog-api.v2"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
)

// ImplNone is a noop datadogclient implementation
type ImplNone struct{}

var _ datadogclient.Component = (*ImplNone)(nil) // ensure: ImplNone implements the interface

// NewNone creates a new noop datadogclient component
func NewNone() datadogclient.Component {
	return &ImplNone{}
}

// QueryMetrics does nothing for the noop datadogclient implementation
func (d *ImplNone) QueryMetrics(_, _ int64, _ string) ([]datadog.Series, error) {
	// noop
	return nil, nil
}

// GetRateLimitStats does nothing for the noop datadogclient implementation
func (d *ImplNone) GetRateLimitStats() map[string]datadog.RateLimit {
	// noop
	return nil
}
