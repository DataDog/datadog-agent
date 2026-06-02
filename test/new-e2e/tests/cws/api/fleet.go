// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// IsProductEnabled returns true if the given product is listed in enabled_products
// for the agent with the specified hostname, using the Fleet Automation API.
func (c *Client) IsProductEnabled(hostname, product string) (bool, error) {
	fleetAPI := datadogV2.NewFleetAutomationApi(c.api)

	opts := datadogV2.NewListFleetAgentsOptionalParameters().
		WithFilter("hostname:" + hostname)

	resp, r, err := fleetAPI.ListFleetAgents(c.ctx, *opts)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return false, fmt.Errorf("fleet automation API request failed: %w", err)
	}

	for _, agent := range resp.Data.Attributes.GetAgents() {
		if agent.GetHostname() != hostname {
			continue
		}
		for _, p := range agent.GetEnabledProducts() {
			if p == product {
				return true, nil
			}
		}
		return false, nil
	}

	return false, fmt.Errorf("no agent found with hostname %s", hostname)
}
