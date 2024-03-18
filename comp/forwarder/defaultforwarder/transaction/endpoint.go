// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	config "github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

// Endpoint is an endpoint
type Endpoint struct {
	// Route to hit in the HTTP transaction
	Route string
	// Name of the endpoint for the telemetry metrics
	Name string
}

// String returns the route of the endpoint
func (e Endpoint) String() string {
	return e.Route
}

// MakeFlareEndpoint Create flare endpoint to the shape of "https://<version>-flare.agent.datadoghq.com/support/flare"
func MakeFlareEndpoint() string {
	url := configUtils.GetInfraEndpoint(config.Datadog)
	baseURL, _ := configUtils.AddAgentVersionToDomain(url, "flare")
	return baseURL + "/support/flare"
}
