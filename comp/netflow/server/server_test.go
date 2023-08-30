// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	nfconfig "github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/testutil"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getAggregator(logger log.Component, lc fx.Lifecycle) aggregator.DemultiplexerWithAggregator {
	agg := aggregator.InitTestAgentDemultiplexerWithFlushInterval(logger, 10*time.Millisecond)
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		agg.Stop(false)
		return nil
	}})
	return agg
}

// testModule is an fx module of common dependencies for all tests
var testModule = fx.Module(
	"ServerTestModule",
	// Provide the test demux/aggregator
	fx.Provide(
		getAggregator,
	),
	// Set the hostname to my-hostname
	fx.Replace(netflowHostname("my-hostname")),
	// Set the internal flush frequency to a small number so tests don't take forever
	fx.Decorate(
		// Allow tests to inject incomplete config and have defaults set automatically
		func(conf *nfconfig.NetflowConfig) (*nfconfig.NetflowConfig, error) {
			return conf, conf.SetDefaults("default")
		},
	),
	fx.Invoke(func(c Component) {
		c.(*Server).FlowAgg.FlushFlowsToSendInterval = 100 * time.Millisecond
	}),
	Module,
)

func TestStartServerAndStopServer(t *testing.T) {
	port := testutil.GetFreePort()

	server := fxutil.Test[Component](t, fx.Options(
		log.MockModule,
		testModule,
		fx.Replace(
			&nfconfig.NetflowConfig{
				Enabled: true,
				Listeners: []nfconfig.ListenerConfig{{
					FlowType: "netflow5",
					BindHost: "127.0.0.1",
					Port:     port,
				}},
			},
		),
	))
	assert.NotNil(t, server)
	assert.True(t, server.(*Server).started)
}
