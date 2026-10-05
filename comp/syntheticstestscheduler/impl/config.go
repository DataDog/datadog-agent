// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const defaultSite = "datadoghq.com"

type schedulerConfigs struct {
	workers                    int
	flushInterval              time.Duration
	syntheticsSchedulerEnabled bool
	// namespace is the default NDM namespace stamped on emitted paths, mirroring
	// the network_path integration. Individual tests may override it.
	namespace string
}

func newSchedulerConfigs(agentConfig config.Component) *schedulerConfigs {
	return &schedulerConfigs{
		syntheticsSchedulerEnabled: agentConfig.GetBool("synthetics.collector.enabled"),
		workers:                    agentConfig.GetInt("synthetics.collector.workers"),
		flushInterval:              agentConfig.GetDuration("synthetics.collector.flush_interval"),
		namespace:                  agentConfig.GetString("network_devices.namespace"),
	}
}

type testPollerConfig struct {
	site          string
	apiKey        string
	agentVersion  string
	httpTransport *http.Transport
}

func newTestPollerConfig(agentConfig config.Component) *testPollerConfig {
	site := agentConfig.GetString("site")
	if site == "" {
		site = defaultSite
	}

	return &testPollerConfig{
		site:          site,
		apiKey:        agentConfig.GetString("api_key"),
		agentVersion:  version.AgentVersion,
		httpTransport: httputils.CreateHTTPTransport(agentConfig),
	}
}
