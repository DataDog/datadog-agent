// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorimplnone "github.com/DataDog/datadog-agent/comp/logs/auditor/impl-none"
	integrationsimpl "github.com/DataDog/datadog-agent/comp/logs/integrations/impl"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func newTestLogAgentForRouting(t *testing.T, cfg model.Reader, eps *config.Endpoints) *logAgent {
	t.Helper()
	return &logAgent{
		log:         logmock.New(t),
		config:      cfg,
		hostname:    hostnameimpl.NewHostnameService(),
		secrets:     secretsnoopimpl.NewComponent().Comp,
		sources:     sources.NewLogSources(),
		endpoints:   eps,
		tracker:     tailers.NewTailerTracker(),
		auditor:     auditorimplnone.NewAuditor(),
		compression: compressionfx.NewMockCompressor(),
	}
}

func TestRoutingConfigDisabledRetainsLegacyProviderAndMRF(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.enabled", false)
	cfg.SetInTest("multi_region_failover.enabled", true)
	cfg.SetInTest("multi_region_failover.failover_logs", true)
	cfg.SetInTest("logs_config.pipelines", 1)

	main := config.NewEndpoint("us1-key", "", "us1.example.com", 443, "", true)
	mrf := config.NewEndpoint("mrf-key", "", "mrf.example.com", 443, "", true)
	mrf.IsMRF = true
	eps := config.NewEndpointsWithBatchSettings(main, []config.Endpoint{mrf}, false, true, time.Second, 2, 100, 1024*1024, 64)

	a := newTestLogAgentForRouting(t, cfg, eps)
	rules, tagFilters, fpCfg, err := a.configureAgent()
	require.NoError(t, err)
	require.Nil(t, a.routingPolicy)

	a.SetupPipeline(rules, tagFilters, option.None[workloadmeta.Component](), integrationsimpl.NewLogsIntegration(), *fpCfg)
	_, isRouted := a.pipelineProvider.(*pipeline.RoutedProvider)
	require.False(t, isRouted, "disabled routing must retain legacy pipeline.Provider")
}

func TestRoutingConfigEnabledNormalSetupAndUnsupportedCombinations(t *testing.T) {
	main := config.NewEndpoint("us1-key", "", "us1.example.com", 443, "", true)
	us5 := config.NewEndpoint("us5-key", "", "us5.example.com", 443, "", true)
	httpEps := config.NewEndpointsWithBatchSettings(main, []config.Endpoint{us5}, false, true, time.Second, 2, 100, 1024*1024, 64)

	t.Run("enabled with HTTP endpoints constructs RoutedProvider with match-all defaults", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.enabled", true)
		cfg.SetInTest("logs_config.routing.max_groups", 4)
		cfg.SetInTest("logs_config.pipelines", 1)

		a := newTestLogAgentForRouting(t, cfg, httpEps)
		rules, tagFilters, fpCfg, err := a.configureAgent()
		require.NoError(t, err)
		require.NotNil(t, a.routingPolicy)
		require.Len(t, a.routingPolicy.Endpoints(), 2)

		a.SetupPipeline(rules, tagFilters, option.None[workloadmeta.Component](), integrationsimpl.NewLogsIntegration(), *fpCfg)
		_, isRouted := a.pipelineProvider.(*pipeline.RoutedProvider)
		require.True(t, isRouted, "enabled routing must construct *pipeline.RoutedProvider")
	})

	t.Run("rejects TCP transport", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.enabled", true)
		tcpEps := config.NewEndpointsWithBatchSettings(main, nil, false, false, time.Second, 2, 100, 1024*1024, 64)
		a := newTestLogAgentForRouting(t, cfg, tcpEps)
		_, _, _, err := a.configureAgent()
		require.Error(t, err)
		require.Contains(t, err.Error(), "HTTP transport")
	})

	t.Run("rejects MRF enabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.enabled", true)
		cfg.SetInTest("multi_region_failover.enabled", true)
		a := newTestLogAgentForRouting(t, cfg, httpEps)
		_, _, _, err := a.configureAgent()
		require.Error(t, err)
		require.Contains(t, err.Error(), "multi_region_failover")
	})

	t.Run("rejects legacy disable_distributed_senders", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.enabled", true)
		cfg.SetInTest("logs_config.disable_distributed_senders", true)
		a := newTestLogAgentForRouting(t, cfg, httpEps)
		_, _, _, err := a.configureAgent()
		require.Error(t, err)
		require.Contains(t, err.Error(), "disable_distributed_senders")
	})

	t.Run("rejects non-empty selector expression via StubCompiler", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.enabled", true)
		a := newTestLogAgentForRouting(t, cfg, httpEps)
		a.routingPolicyBuilder = func(eps *config.Endpoints) (*routing.EndpointPolicy, error) {
			return routing.NewEndpointPolicy(routing.PolicyOptions{
				Endpoints: eps,
				Selectors: map[string]string{routing.AdditionalEndpointID(0): `severity == "notice"`},
				Compiler:  routing.StubCompiler{},
			})
		}
		_, _, _, err := a.configureAgent()
		require.ErrorIs(t, err, routing.ErrOTTLNotImplemented)
	})
}
