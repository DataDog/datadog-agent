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
	"go.uber.org/fx/fxtest"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"

	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"

	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
)

type dummyFlowProcessor struct {
	receivedMessages chan interface{}
	stopped          bool
}

func (d *dummyFlowProcessor) FlowRoutine(workers int, addr string, port int, reuseport bool) error { //nolint:revive // TODO fix revive unused-parameter
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

// testOptions is an fx collection of common dependencies for all tests
var testOptions = fx.Options(
	Module(),
	nfconfig.MockModule(),
	forwarderimpl.MockModule(),
	hostnameimpl.MockModule(),
	logimpl.MockModule(),
	demultiplexerimpl.MockModule(),
	defaultforwarder.MockModule(),
	config.MockModule(),
	fx.Invoke(func(lc fx.Lifecycle, c Component) {
		// Set the internal flush frequency to a small number so tests don't take forever
		c.(*Server).FlowAgg.FlushFlowsToSendInterval = 100 * time.Millisecond
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				// Remove the flow processor to avoid a spurious race detection error
				replaceWithDummyFlowProcessor(c.(*Server))
				return nil
			},
		})
	}),
)

func TestStartServerAndStopServer(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var component Component
	app := fxtest.New(t, fx.Options(
		testOptions,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
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
		fx.Populate(&component),
	))
	server := component.(*Server)
	assert.NotNil(t, server)
	assert.False(t, server.running)
	app.RequireStart()
	assert.True(t, server.running)
	app.RequireStop()
	assert.False(t, server.running)
}
