// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
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
	panic("not called")
}

func (c *CLCRunnerClient) init() {
	panic("not called")
}

// GetVersion fetches the version of the CLC Runner
func (c *CLCRunnerClient) GetVersion(IP string) (version.Version, error) {
	panic("not called")
}

// GetRunnerStats fetches the runner stats exposed by the Cluster Level Check Runner
func (c *CLCRunnerClient) GetRunnerStats(IP string) (types.CLCRunnersStats, error) {
	panic("not called")
}

// GetRunnerWorkers fetches the runner workers information exposed by the Cluster Level Check Runner
func (c *CLCRunnerClient) GetRunnerWorkers(IP string) (types.Workers, error) {
	panic("not called")
}

// init globalCLCRunnerClient
func init() {
	globalCLCRunnerClient = &CLCRunnerClient{}
}
