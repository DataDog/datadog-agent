// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ndmtmp

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/aggregator"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/sender"
	ddagg "github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the ndmtmp components, since this is not done
		// automatically.
		fx.Invoke(func(forwarder.Component) {}),
		fx.Invoke(func(sender.Component) {}),
		fx.Invoke(func(aggregator.Component) {}),
		Bundle,
		fx.Provide(func() *ddagg.AgentDemultiplexer {
			return &ddagg.AgentDemultiplexer{}
		}),
	))
}

func TestMockBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		log.MockModule,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Invoke(func(forwarder.Component) {}),
		fx.Invoke(func(sender.Component) {}),
		fx.Invoke(func(aggregator.Component) {}),
		MockBundle,
	))
}
