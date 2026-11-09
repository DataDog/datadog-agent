// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"

	statsdcomp "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
)

// NewMetricsClient builds a DogStatsD client from the Agent's configured
// host/port endpoint.
func NewMetricsClient(config Config, statsdComp statsdcomp.Component) (statsd.ClientInterface, error) {
	host, port := config.MetricsAddress()
	client, err := statsdComp.CreateForHostPort(host, port)
	if err != nil {
		return &statsd.NoOpClient{}, fmt.Errorf("failed to create DogStatsD client: %w", err)
	}
	return client, nil
}
