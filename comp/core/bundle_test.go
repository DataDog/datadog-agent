// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package core

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the core components, since this is not done
		// automatically.
		fx.Invoke(func(config.Component) {}),
		fx.Invoke(func(log.Component) {}),
		fx.Invoke(func(flare.Component) {}),

		fx.Supply(BundleParams{}),
		Bundle))
}

func TestMockBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),

		// instantiate all of the core components, since this is not done
		// automatically.
		fx.Invoke(func(config.Component) {}),
		fx.Invoke(func(log.Component) {}),

		fx.Supply(BundleParams{}),
		MockBundle))
}

func TestCreateSecurityAgentBundleParams(t *testing.T) {
	bundleParams := CreateSecurityAgentBundleParams([]string{})

	require.Equal(t, true, bundleParams.ConfigLoadSecurityAgent, "ConfigLoadSecurityAgent values not matching")
	require.Equal(t, common.DefaultConfPath, bundleParams.DefaultConfPath, "DefaultConfPath values not matching")
}
