// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	dcaEndpointsChecksPath        = "api/v1/endpointschecks"
	dcaEndpointsChecksConfigsPath = dcaEndpointsChecksPath + "/configs"
)

// GetEndpointsCheckConfigs is called by the endpointscheck config provider
func (c *DCAClient) GetEndpointsCheckConfigs(ctx context.Context, nodeName string) (types.ConfigResponse, error) {
	// Retry on the main URL if the leader fails
	willRetry := c.leaderClient.hasLeader()

	result, err := c.doGetEndpointsCheckConfigs(ctx, nodeName)
	if err != nil && willRetry {
		log.Debugf("Got error on leader, retrying via the service: %s", err)
		c.leaderClient.resetURL()
		return c.doGetEndpointsCheckConfigs(ctx, nodeName)
	}
	return result, err
}

func (c *DCAClient) doGetEndpointsCheckConfigs(ctx context.Context, nodeName string) (types.ConfigResponse, error) {
	var configs types.ConfigResponse
	var err error

	// https://host:port/api/v1/endpointschecks/configs/{nodeName}
	rawURL := c.leaderClient.buildURL(dcaEndpointsChecksConfigsPath, nodeName)
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return configs, err
	}
	req.Header = c.clusterAgentAPIRequestHeaders

	resp, err := c.leaderClient.Do(req)
	if err != nil {
		return configs, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return configs, fmt.Errorf("unexpected response: %d - %s", resp.StatusCode, resp.Status)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return configs, err
	}
	err = json.Unmarshal(b, &configs)
	return configs, err
}
