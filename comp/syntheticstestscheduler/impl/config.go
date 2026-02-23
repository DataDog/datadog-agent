// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

type schedulerConfigs struct {
	workers                    int
	flushInterval              time.Duration
	syntheticsSchedulerEnabled bool
}

func newSchedulerConfigs(agentConfig config.Component) *schedulerConfigs {
	return &schedulerConfigs{
		syntheticsSchedulerEnabled: agentConfig.GetBool("synthetics.collector.enabled"),
		workers:                    agentConfig.GetInt("synthetics.collector.workers"),
		flushInterval:              agentConfig.GetDuration("synthetics.collector.flush_interval"),
	}
}

type onDemandPollerConfig struct {
	site          string
	apiKey        string
	httpTransport *http.Transport
}

func newOnDemandPollerConfig(agentConfig config.Component) *onDemandPollerConfig {
	site := agentConfig.GetString("site")
	if site == "" {
		site = pkgconfigsetup.DefaultSite
	}

	return &onDemandPollerConfig{
		site:          site,
		apiKey:        agentConfig.GetString("api_key"),
		httpTransport: httputils.CreateHTTPTransport(agentConfig),
	}
}
