// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	statsdcomp "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

// NewMetricsClient builds a DogStatsD client from the Agent's configured
// host/port endpoint.
func NewMetricsClient(config config.Component, statsdComp statsdcomp.Component) (statsd.ClientInterface, error) {
	port := config.GetInt("dogstatsd_port")
	client, err := statsdComp.CreateForHostPort(configutils.GetBindHost(config), port)
	if err != nil {
		return &statsd.NoOpClient{}, fmt.Errorf("failed to create DogStatsD client: %w", err)
	}
	return client, nil
}
