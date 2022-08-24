// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultGraceDuration = 60 * time.Second

// ClusterChecksConfigProvider implements the ConfigProvider interface
// for the cluster check feature.
type ClusterChecksConfigProvider struct {
	dcaClient        clusteragent.DCAClientInterface
	graceDuration    time.Duration
	degradedDuration time.Duration
	heartbeat        time.Time
	lastChange       int64
	identifier       string
	flushedConfigs   bool
}

// NewClusterChecksConfigProvider returns a new ConfigProvider collecting
// cluster check configurations from the cluster-agent.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewClusterChecksConfigProvider(providerConfig *config.ConfigurationProviders) (ConfigProvider, error) {
	if providerConfig == nil {
		providerConfig = &config.ConfigurationProviders{}
	}

	c := &ClusterChecksConfigProvider{
		graceDuration:    defaultGraceDuration,
		degradedDuration: defaultDegradedDeadline,
	}

	c.identifier = config.Datadog.GetString("clc_runner_id")
	if c.identifier == "" {
		c.identifier, _ = hostname.Get(context.TODO())
		if config.Datadog.GetBool("cloud_foundry") {
			boshID := config.Datadog.GetString("bosh_id")
			if boshID == "" {
				log.Warn("configuration variable cloud_foundry is set to true, but bosh_id is empty, can't retrieve node name")
			} else {
				c.identifier = boshID
			}
		}
	}

	if providerConfig.GraceTimeSeconds > 0 {
		c.graceDuration = time.Duration(providerConfig.GraceTimeSeconds) * time.Second
	}

	if providerConfig.DegradedDeadlineMinutes > 0 {
		c.degradedDuration = time.Duration(providerConfig.DegradedDeadlineMinutes) * time.Minute
	}

	// Register in the cluster agent as soon as possible
	c.IsUpToDate(context.TODO()) //nolint:errcheck

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
	return names.ClusterChecks
}

func (c *ClusterChecksConfigProvider) withinGracePeriod() bool {
	return c.heartbeat.Add(c.graceDuration).After(time.Now())
}

func (c *ClusterChecksConfigProvider) withinDegradedModePeriod() bool {
	return withinDegradedModePeriod(c.heartbeat, c.degradedDuration)
}

// IsUpToDate queries the cluster-agent to update its status and
// query if new configurations are available
func (c *ClusterChecksConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	if c.dcaClient == nil {
		err := c.initClient()
		if err != nil {
			return false, err
		}
	}

	status := types.NodeStatus{
		LastChange: c.lastChange,
	}

	reply, err := c.dcaClient.PostClusterCheckStatus(ctx, c.identifier, status)
	if err != nil {
		if c.withinGracePeriod() {
			// Return true to keep the configs during the grace period
			log.Debugf("Catching error during grace period: %s", err)
			return true, nil
		}
		// Return false, the next Collect will flush the configs
		return false, err
	}

	c.heartbeat = time.Now()
	if reply.IsUpToDate {
		log.Tracef("Up to date with change %d", c.lastChange)
	} else {
		log.Tracef("Not up to date with change %d", c.lastChange)
	}
	return reply.IsUpToDate, nil
}

// Collect retrieves configurations the cluster-agent dispatched to this agent
func (c *ClusterChecksConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	if c.dcaClient == nil {
		err := c.initClient()
		if err != nil {
			return nil, err
		}
	}

	reply, err := c.dcaClient.GetClusterCheckConfigs(ctx, c.identifier)
	if err != nil {
		if (errors.IsRemoteService(err) || errors.IsTimeout(err)) && c.withinDegradedModePeriod() {
			// Degraded mode: return the error to keep the configs scheduled
			// during a Cluster Agent / network outage
			return nil, err
		}

		if !c.flushedConfigs {
			// On first error after grace period, mask the error once
			// to delete the configurations and de-schedule the checks
			// Returning nil, nil here unschedules the checks when the grace period
			// and the degraded mode deadline are both exceeded.
			c.flushedConfigs = true
			return nil, nil
		}

		return nil, err
	}

	c.flushedConfigs = false
	c.lastChange = reply.LastChange
	c.heartbeat = time.Now()
	log.Tracef("Storing last change %d", c.lastChange)

	return reply.Configs, nil
}

func init() {
	RegisterProvider(names.ClusterChecksRegisterName, NewClusterChecksConfigProvider)
}

// GetConfigErrors is not implemented for the ClusterChecksConfigProvider
func (c *ClusterChecksConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
