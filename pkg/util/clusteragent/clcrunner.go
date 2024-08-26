// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

/*
Client to query the Datadog Cluster Level Check Runner API.
*/

const (
	clcRunnerPath        = "api/v1/clcrunner"
	clcRunnerVersionPath = "version"
	clcRunnerStatsPath   = "stats"
	clcRunnerWorkersPath = "workers"
)

var globalCLCRunnerClient *CLCRunnerClient

// CLCRunnerClientInterface is required to query the API of Datadog Cluster Level Check Runner
type CLCRunnerClientInterface interface {
	GetVersion(IP string) (version.Version, error)
	GetRunnerStats(IP string) (types.CLCRunnersStats, error)
	GetRunnerWorkers(IP string) (types.Workers, error)
}

// CLCRunnerClient is required to query the API of Datadog Cluster Level Check Runner
type CLCRunnerClient struct {
	sync.Once
	initErr                    error
	clcRunnerAPIRequestHeaders http.Header
	clcRunnerAPIClient         *http.Client
	clcRunnerPort              int
}

// GetCLCRunnerClient returns or init the CLCRunnerClient
func GetCLCRunnerClient() (CLCRunnerClientInterface, error) {
	globalCLCRunnerClient.Do(globalCLCRunnerClient.init)
	return globalCLCRunnerClient, globalCLCRunnerClient.initErr
}

func (c *CLCRunnerClient) init() {
	c.initErr = nil

	authToken, err := security.GetClusterAgentAuthToken(config.Datadog())
	if err != nil {
		c.initErr = err
		return
	}

	// Set headers
	c.clcRunnerAPIRequestHeaders = http.Header{}
	c.clcRunnerAPIRequestHeaders.Set(authorizationHeaderKey, fmt.Sprintf("Bearer %s", authToken))

	// Set http client
	// TODO remove insecure
	c.clcRunnerAPIClient = util.GetClient(false)
	c.clcRunnerAPIClient.Timeout = 2 * time.Second

	// Set http port used by the CLC Runners
	c.clcRunnerPort = config.Datadog().GetInt("cluster_checks.clc_runners_port")
}

// GetVersion fetches the version of the CLC Runner
func (c *CLCRunnerClient) GetVersion(IP string) (version.Version, error) {
	var version version.Version
	var err error

	rawURL := fmt.Sprintf("https://%s:%d/%s/%s", IP, c.clcRunnerPort, clcRunnerPath, clcRunnerVersionPath)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return version, err
	}
	req.Header = c.clcRunnerAPIRequestHeaders

	resp, err := c.clcRunnerAPIClient.Do(req)
	if err != nil {
		return version, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return version, fmt.Errorf("unexpected status code from CLC runner: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return version, err
	}

	err = json.Unmarshal(body, &version)

	return version, err
}

// GetRunnerStats fetches the runner stats exposed by the Cluster Level Check Runner
func (c *CLCRunnerClient) GetRunnerStats(IP string) (types.CLCRunnersStats, error) {
	var stats types.CLCRunnersStats
	var err error

	rawURL := fmt.Sprintf("https://%s:%d/%s/%s", IP, c.clcRunnerPort, clcRunnerPath, clcRunnerStatsPath)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return stats, err
	}
	req.Header = c.clcRunnerAPIRequestHeaders

	resp, err := c.clcRunnerAPIClient.Do(req)
	if err != nil {
		return stats, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return stats, fmt.Errorf("unexpected status code from CLC runner: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return stats, err
	}

	err = json.Unmarshal(body, &stats)

	// clean stats map if it contains an empty entry
	delete(stats, "")

	return stats, err
}

// GetRunnerWorkers fetches the runner workers information exposed by the Cluster Level Check Runner
func (c *CLCRunnerClient) GetRunnerWorkers(IP string) (types.Workers, error) {
	var workers types.Workers

	rawURL := fmt.Sprintf("https://%s:%d/%s/%s", IP, c.clcRunnerPort, clcRunnerPath, clcRunnerWorkersPath)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return workers, err
	}
	req.Header = c.clcRunnerAPIRequestHeaders

	resp, err := c.clcRunnerAPIClient.Do(req)
	if err != nil {
		return workers, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return workers, fmt.Errorf("unexpected status code from CLC runner: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return workers, err
	}

	err = json.Unmarshal(body, &workers)

	return workers, err
}

// init globalCLCRunnerClient
func init() {
	globalCLCRunnerClient = &CLCRunnerClient{}
}
