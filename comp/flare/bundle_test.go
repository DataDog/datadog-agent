// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/flare/flare"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the flare components, since this is not done
		// automatically.
		fx.Invoke(func(flare.Component) {}),
		fx.Supply(core.BundleParams{}),
		core.Bundle,

		Bundle))
}

func TestMockBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),

		// instantiate all of the flare components, since this is not done
		// automatically.
		fx.Invoke(func(flare.Component) {}),
		fx.Supply(core.BundleParams{}),
		core.Bundle,

		MockBundle))
}
