// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EndpointsChecksConfigProvider implements the ConfigProvider interface
// for the endpoints check feature.
type EndpointsChecksConfigProvider struct {
	dcaClient        clusteragent.DCAClientInterface
	degradedDuration time.Duration
	heartbeat        time.Time
	nodeName         string
	flushedConfigs   bool
}

// NewEndpointsChecksConfigProvider returns a new ConfigProvider collecting
// endpoints check configurations from the cluster-agent.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewEndpointsChecksConfigProvider(providerConfig *config.ConfigurationProviders) (ConfigProvider, error) {
	c := &EndpointsChecksConfigProvider{
		degradedDuration: defaultDegradedDeadline,
	}

	if providerConfig.DegradedDeadlineMinutes > 0 {
		c.degradedDuration = time.Duration(providerConfig.DegradedDeadlineMinutes) * time.Minute
	}

	var err error
	c.nodeName, err = getNodename(context.TODO())
	if err != nil {
		log.Errorf("Cannot get node name: %s", err)
		return nil, err
	}
	err = c.initClient()
	if err != nil {
		log.Warnf("Cannot get dca client: %v", err)
	}
	return c, nil
}

// String returns a string representation of the EndpointsChecksConfigProvider
func (c *EndpointsChecksConfigProvider) String() string {
	return names.EndpointsChecks
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to Kubernetes's data.
func (c *EndpointsChecksConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return false, nil
}

func (c *EndpointsChecksConfigProvider) withinDegradedModePeriod() bool {
	return withinDegradedModePeriod(c.heartbeat, c.degradedDuration)
}

// Collect retrieves endpoints checks configurations the cluster-agent dispatched to this agent
func (c *EndpointsChecksConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	if c.dcaClient == nil {
		err := c.initClient()
		if err != nil {
			return nil, err
		}
	}
	reply, err := c.dcaClient.GetEndpointsCheckConfigs(ctx, c.nodeName)
	if err != nil {
		if (errors.IsRemoteService(err) || errors.IsTimeout(err)) && c.withinDegradedModePeriod() {
			// Degraded mode: return true to keep the configs scheduled
			// during a Cluster Agent / network outage
			return nil, err
		}

		if !c.flushedConfigs {
			// On first error after grace period, mask the error once
			// to delete the configurations and de-schedule the checks
			// Returning nil, nil here unschedules the checks when the
			// the degraded mode deadline is exceeded.
			c.flushedConfigs = true
			return nil, nil
		}

		return nil, err
	}

	c.flushedConfigs = false
	c.heartbeat = time.Now()

	return reply.Configs, nil
}

// getNodename retrieves current node name from kubelet (if running on Kubernetes)
// or bosh ID of current node (if running on Cloud Foundry).
func getNodename(ctx context.Context) (string, error) {
	if config.Datadog.GetBool("cloud_foundry") {
		boshID := config.Datadog.GetString("bosh_id")
		if boshID == "" {
			return "", fmt.Errorf("configuration variable cloud_foundry is set to true, but bosh_id is empty, can't retrieve node name")
		}
		return boshID, nil
	}
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		log.Errorf("Cannot get kubeUtil object: %s", err)
		return "", err
	}
	return ku.GetNodename(ctx)
}

// initClient initializes a cluster agent client.
func (c *EndpointsChecksConfigProvider) initClient() error {
	dcaClient, err := clusteragent.GetClusterAgentClient()
	if err == nil {
		c.dcaClient = dcaClient
	}
	return err
}

func init() {
	RegisterProvider(names.EndpointsChecksRegisterName, NewEndpointsChecksConfigProvider)
}

// GetConfigErrors is not implemented for the EndpointsChecksConfigProvider
func (c *EndpointsChecksConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
