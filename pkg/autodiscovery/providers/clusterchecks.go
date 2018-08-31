// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultGraceDuration = 60 * time.Second

// ClusterChecksConfigProvider implements the ConfigProvider interface
// for the cluster check feature.
type ClusterChecksConfigProvider struct {
	dcaClient     *clusteragent.DCAClient
	graceDuration time.Duration
	lastPing      time.Time
	lastChange    int64
	nodeName      string
}

// NewClusterChecksConfigProvider returns a new ConfigProvider collecting
// cluster check configurations from the cluster-agent.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewClusterChecksConfigProvider(cfg config.ConfigurationProviders) (ConfigProvider, error) {
	c := &ClusterChecksConfigProvider{
		graceDuration: defaultGraceDuration,
	}

	c.nodeName, _ = util.GetHostname()
	if cfg.GraceTimeSeconds > 0 {
		c.graceDuration = time.Duration(cfg.GraceTimeSeconds) * time.Second
	}

	return c, nil
}

func (c *ClusterChecksConfigProvider) initClient() error {
	dcaClient, err := clusteragent.GetClusterAgentClient()
	if err == nil {
		c.dcaClient = dcaClient
	}
	return err
}

// String returns a string representation of the ClusterChecksConfigProvider
func (c *ClusterChecksConfigProvider) String() string {
	return ClusterChecks
}

func (c *ClusterChecksConfigProvider) withinGracePeriod() bool {
	return c.lastPing.Add(c.graceDuration).After(time.Now())
}

// IsUpToDate queries the cluster-agent to update its status and
// query if new configurations are available
func (c *ClusterChecksConfigProvider) IsUpToDate() (bool, error) {
	if c.dcaClient == nil {
		err := c.initClient()
		if err != nil {
			return false, err
		}
	}

	status := types.NodeStatus{
		LastChange: c.lastChange,
	}

	reply, err := c.dcaClient.PostClusterCheckStatus(c.nodeName, status)
	if err != nil {
		if c.withinGracePeriod() {
			log.Debug("Cannot reach DCA, but still within grace time, keeping config: %s", err)
			return true, nil
		}
		log.Debug("Cannot reach DCA, but grace time elapsed, purging config: %s", err)
		return false, nil
	}

	c.lastPing = time.Now()
	return reply.IsUpToDate, nil
}

// Collect retrieves configurations the cluster-agent dispatched to this agent
func (c *ClusterChecksConfigProvider) Collect() ([]integration.Config, error) {
	if c.dcaClient == nil {
		err := c.initClient()
		if err != nil {
			return nil, err
		}
	}

	reply, err := c.dcaClient.GetClusterCheckConfigs(c.nodeName)
	if err != nil {
		if c.withinGracePeriod() {
			// Bubble-up the error to keep the known configurations
			return nil, err
		}
		// Catch the error to flush the configurations
		log.Debug("Cannot reach DCA, but grace time elapsed, purging config: %s", err)
		return nil, err
	}

	c.lastChange = reply.LastChange
	return reply.Configs, nil
}

func init() {
	RegisterProvider("clusterchecks", NewClusterChecksConfigProvider)
}
