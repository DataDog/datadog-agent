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

	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/forwarder"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
	"github.com/DataDog/datadog-agent/comp/netflow/hostname"
	"github.com/DataDog/datadog-agent/comp/netflow/sender"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dummyFlowProcessor struct {
	receivedMessages chan interface{}
	stopped          bool
}

func (d *dummyFlowProcessor) FlowRoutine(workers int, addr string, port int, reuseport bool) error {
	return utils.UDPStoppableRoutine(make(chan struct{}), "test_udp", func(msg interface{}) error {
		d.receivedMessages <- msg
		return nil
	}, 3, addr, port, false, logrus.StandardLogger())
}

func (d *dummyFlowProcessor) Shutdown() {
	d.stopped = true
}

func replaceWithDummyFlowProcessor(server *Server) *dummyFlowProcessor {
	// Testing using a dummyFlowProcessor since we can't test using real goflow flow processor
	// due to this race condition https://github.com/netsampler/goflow2/issues/83
	flowProcessor := &dummyFlowProcessor{}
	listener := server.listeners[0]
	listener.flowState = &goflowlib.FlowStateWrapper{
		State:    flowProcessor,
		Hostname: "abc",
		Port:     0,
	}
	return flowProcessor
}

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
	Module,
	nfconfig.MockModule,
	sender.Module,
	forwarder.MockModule,
	hostname.MockModule,
	fx.Provide(
		getAggregator,
	),
	fx.Invoke(func(lc fx.Lifecycle, c Component) {
		// Set the internal flush frequency to a small number so tests don't take forever
		c.(*Server).FlowAgg.FlushFlowsToSendInterval = 100 * time.Millisecond
	}),
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
	)).(*Server)
	assert.NotNil(t, server)
	assert.False(t, server.running)
	require.NoError(t, server.Start())
	assert.True(t, server.running)
	replaceWithDummyFlowProcessor(server)
	server.Stop()
	assert.False(t, server.running)
}
