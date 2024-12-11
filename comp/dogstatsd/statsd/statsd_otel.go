// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build otlp

package statsd

import (
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
)

type otelcomponent struct {
	client *metricsclient.StatsdClientWrapper
}

// NewOTelStatsd returns a new statsd component for the OTel agent
func NewOTelStatsd(client *metricsclient.StatsdClientWrapper) Component {
	return &otelcomponent{client}
}

// Get returns a pre-configured and shared statsd client (requires STATSD_URL env var to be set)
func (m *otelcomponent) Get() (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

// Create returns a pre-configured statsd client
func (m *otelcomponent) Create(_ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

// GetForAddr returns a pre-configured statsd -client that defaults to `addr` if no env var is set
func (m *otelcomponent) CreateForAddr(_ string, _ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

// GetForHostPort returns a pre-configured statsd client that defaults to `host:port` if no env var is set
func (m *otelcomponent) CreateForHostPort(_ string, _ int, _ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

var _ Component = (*otelcomponent)(nil)
