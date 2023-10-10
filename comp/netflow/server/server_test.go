// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/aggregator"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/sender"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
)

// testOptions is an fx collection of common dependencies for all tests
var testOptions = fx.Options(
	Module,
	nfconfig.MockModule,
	sender.Module,
	forwarder.MockModule,
	hostname.MockModule,
	log.MockModule,
	aggregator.MockModule,
	fx.Invoke(func(lc fx.Lifecycle, c Component) {
		// Set the internal flush frequency to a small number so tests don't take forever
		c.(*Server).FlowAgg.FlushFlowsToSendInterval = 100 * time.Millisecond
	}),
)

func TestStartServerAndStopServer(t *testing.T) {
	port := testutil.GetFreePort()
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
