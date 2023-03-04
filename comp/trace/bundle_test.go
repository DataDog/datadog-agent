// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the core components, since this is not done
		// automatically.
		fx.Invoke(func(r config.Component) {}),
		fx.Invoke(func(coreconfig.Component) {}),
		// supply the necessary parameters to populate the agent and trace
		// configs in the agent.
		fx.Supply(config.NewParams()),
		fx.Supply(coreconfig.Params{}),
		Bundle))
}

func TestBundle(t *testing.T) {
	os.Setenv("DD_APP_KEY", "abc1234")
	defer func() { os.Unsetenv("DD_APP_KEY") }()

	os.Setenv("DD_DD_URL", "https://example.com")
	defer func() { os.Unsetenv("DD_DD_URL") }()

	fxutil.Test(t, fx.Options(
		// instantiate all of the core components, since this is not done
		// automatically.
		fx.Invoke(func(r config.Component) {}),
		fx.Invoke(func(coreconfig.Component) {}),
		// supply the necessary parameters to populate the agent and trace
		// configs in the agent.
		fx.Supply(config.NewParams()),
		fx.Supply(coreconfig.Params{}),
		MockBundle,
	), func(config config.Component) {
		cfg := config.Object()

		require.NotNil(t, cfg)
	})
}
