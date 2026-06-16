// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InstrumentationChecksConfigProvider implements the ConfigProvider interface
// for the instrumentation checks feature. It pulls AD configurations derived
// from DatadogInstrumentation CRs from the cluster agent.
type InstrumentationChecksConfigProvider struct {
	dcaClient        clusteragent.InstrumentationCheckClient
	degradedDuration time.Duration
	heartbeat        time.Time
	flushedConfigs   bool
	configHash       uint64
}

// NewInstrumentationChecksConfigProvider returns a new ConfigProvider collecting
// instrumentation check configurations from the cluster-agent.
func NewInstrumentationChecksConfigProvider(providerConfig *pkgconfigsetup.ConfigurationProviders, _ *telemetry.Store) (types.ConfigProvider, error) {
	c := &InstrumentationChecksConfigProvider{
		degradedDuration: defaultDegradedDeadline,
	}

	if providerConfig.DegradedDeadlineMinutes > 0 {
		c.degradedDuration = time.Duration(providerConfig.DegradedDeadlineMinutes) * time.Minute
	}

	if err := c.initClient(); err != nil {
		log.Warnf("Cannot get dca client: %v", err)
	}
	return c, nil
}

// String returns a string representation of the InstrumentationChecksConfigProvider
func (c *InstrumentationChecksConfigProvider) String() string {
	return names.InstrumentationChecks
}

// IsUpToDate polls the cluster agent's /instrumentation/status endpoint and
// compares the returned config hash against the provider's last known hash.
// Returns true (skip Collect) only when the cluster agent confirms no new changes.
func (c *InstrumentationChecksConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	if c.dcaClient == nil {
		return false, nil
	}

	// If configs were flushed due to a transient error, force a Collect so checks
	// get rescheduled once the cluster agent recovers — even if the config hash
	// hasn't changed (no config mutations occurred during the outage).
	if c.flushedConfigs {
		return false, nil
	}

	status, err := c.dcaClient.GetInstrumentationStatus(ctx)
	if err != nil {
		// On error, fall through to Collect so the provider can handle degraded mode.
		return false, nil
	}
	return status.ConfigHash == c.configHash, nil
}

func (c *InstrumentationChecksConfigProvider) withinDegradedModePeriod() bool {
	return withinDegradedModePeriod(c.heartbeat, c.degradedDuration)
}

// Collect retrieves instrumentation check configurations from the cluster-agent.
func (c *InstrumentationChecksConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	log.Debugf("Collecting instrumentation check configurations from the cluster-agent")
	if c.dcaClient == nil {
		if err := c.initClient(); err != nil {
			return nil, err
		}
	}
	reply, err := c.dcaClient.GetInstrumentationConfigs(ctx)
	if err != nil {
		if (errors.IsRemoteService(err) || errors.IsTimeout(err)) && c.withinDegradedModePeriod() {
			return nil, err
		}

		// Return nil configs once to signal the scheduler to unschedule
		// checks that may be based on stale configuration.
		if !c.flushedConfigs {
			c.flushedConfigs = true
			return nil, nil
		}

		return nil, err
	}

	log.Debugf("Received %d instrumentation check configurations from the cluster-agent", len(reply.Configs))
	for i := range reply.Configs {
		reply.Configs[i].Provider = names.InstrumentationChecks
	}

	// Reset flush state and update heartbeat so the degraded mode window
	// restarts from this successful response.
	c.flushedConfigs = false
	c.heartbeat = time.Now()
	c.configHash = reply.ConfigHash

	return reply.Configs, nil
}

func (c *InstrumentationChecksConfigProvider) initClient() error {
	dcaClient, err := clusteragent.GetClusterAgentClient()
	if err == nil {
		c.dcaClient = dcaClient
	}
	return err
}

// GetConfigErrors is not implemented for the InstrumentationChecksConfigProvider
func (c *InstrumentationChecksConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}
