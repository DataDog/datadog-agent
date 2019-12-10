// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package clusteragent

import (
	"bytes"
	"fmt"
	"github.com/segmentio/encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	dcaClusterChecksPath        = "api/v1/clusterchecks"
	dcaClusterChecksStatusPath  = dcaClusterChecksPath + "/status"
	dcaClusterChecksConfigsPath = dcaClusterChecksPath + "/configs"
)

// PostClusterCheckStatus is called by the clustercheck config provider
func (c *DCAClient) PostClusterCheckStatus(nodeName string, status types.NodeStatus) (types.StatusResponse, error) {
	// Retry on the main URL if the leader fails
	willRetry := c.leaderClient.hasLeader()

	result, err := c.doPostClusterCheckStatus(nodeName, status)
	if err != nil && willRetry {
		log.Debugf("Got error on leader, retrying via the service: %s", err)
		c.leaderClient.resetURL()
		return c.doPostClusterCheckStatus(nodeName, status)
	}
	return result, err
}

func (c *DCAClient) doPostClusterCheckStatus(nodeName string, status types.NodeStatus) (types.StatusResponse, error) {
	var response types.StatusResponse

	queryBody, err := json.Marshal(status)
	if err != nil {
		return response, err
	}

	// https://host:port/api/v1/clusterchecks/status/{nodeName}
	rawURL := c.leaderClient.buildURL(dcaClusterChecksStatusPath, nodeName)
	req, err := http.NewRequest("POST", rawURL, bytes.NewBuffer(queryBody))
	if err != nil {
		return response, err
	}
	req.Header = c.clusterAgentAPIRequestHeaders

	resp, err := c.leaderClient.Do(req)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("unexpected response: %d - %s", resp.StatusCode, resp.Status)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return response, err
	}
	err = json.Unmarshal(b, &response)
	return response, err
}

// GetClusterCheckConfigs is called by the clustercheck config provider
func (c *DCAClient) GetClusterCheckConfigs(nodeName string) (types.ConfigResponse, error) {
	// Retry on the main URL if the leader fails
	willRetry := c.leaderClient.hasLeader()

	result, err := c.doGetClusterCheckConfigs(nodeName)
	if err != nil && willRetry {
		log.Debugf("Got error on leader, retrying via the service: %s", err)
		c.leaderClient.resetURL()
		return c.doGetClusterCheckConfigs(nodeName)
	}
	return result, err
}

func (c *DCAClient) doGetClusterCheckConfigs(nodeName string) (types.ConfigResponse, error) {
	var configs types.ConfigResponse
	var err error

	// https://host:port/api/v1/clusterchecks/configs/{nodeName}
	rawURL := c.leaderClient.buildURL(dcaClusterChecksConfigsPath, nodeName)
	req, err := http.NewRequest("GET", rawURL, nil)
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
