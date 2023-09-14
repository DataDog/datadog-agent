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

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// TODO(AIT-8301): Why are these tests green if not all elements are supplied?
func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the core components, since this is not done
		// automatically.
		fx.Supply(config.Params{}),
		// supply the necessary parameters to populate the agent and trace
		// configs in the agent.
		config.Module,
		Bundle))
}

func TestMockBundleDependencies(t *testing.T) {
	os.Setenv("DD_APP_KEY", "abc1234")
	defer func() { os.Unsetenv("DD_APP_KEY") }()

	os.Setenv("DD_DD_URL", "https://example.com")
	defer func() { os.Unsetenv("DD_DD_URL") }()

	config := fxutil.Test[config.Component](t, fx.Options(
		// instantiate all of the core components, since this is not done
		// automatically.
		fx.Supply(config.Params{}),
		// supply the necessary parameters to populate the agent and trace
		// configs in the agent.
		config.MockModule,
		MockBundle,
	))
	cfg := config.Object()

	require.NotNil(t, cfg)
}
