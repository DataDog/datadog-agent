// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/comp/trace/agent"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the core components, since this is not done
		// automatically. Use fx.Invoke to make sure components are initialized
		// and all the providers are called.
		fx.Provide(func() context.Context { return context.TODO() }),
		fx.Supply(coreconfig.Params{}),
		coreconfig.Module,
		fx.Invoke(func(_ config.Component) {}),
		fx.Supply(&agent.Params{}),
		fx.Invoke(func(_ agent.Component) {}),
		Bundle))
}

func TestMockBundleDependencies(t *testing.T) {
	os.Setenv("DD_APP_KEY", "abc1234")
	defer func() { os.Unsetenv("DD_APP_KEY") }()

	os.Setenv("DD_DD_URL", "https://example.com")
	defer func() { os.Unsetenv("DD_DD_URL") }()

	cfg := fxutil.Test[config.Component](t, fx.Options(
		fx.Provide(func() context.Context { return context.TODO() }),
		fx.Supply(coreconfig.Params{}),
		coreconfig.MockModule,
		fx.Invoke(func(_ config.Component) {}),
		fx.Supply(&agent.Params{}),
		fx.Invoke(func(_ agent.Component) {}),
		MockBundle,
	))

	require.NotNil(t, cfg.Object())
}
