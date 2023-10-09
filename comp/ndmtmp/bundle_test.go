// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ndmtmp

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		// instantiate all of the ndmtmp components, since this is not done
		// automatically.
		fx.Invoke(func(forwarder.Component) {}),
		demultiplexer.MockModule,
		log.MockModule,
		config.MockModule,
		defaultforwarder.MockModule,
		Bundle,
	))
}

func TestMockBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		log.MockModule,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Invoke(func(forwarder.Component) {}),
		demultiplexer.MockModule,
		MockBundle,
	))
}
