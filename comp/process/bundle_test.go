// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	configComp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var mockCoreBundleParams = core.BundleParams{
	ConfigParams: configComp.NewParams("", configComp.WithConfigMissingOK(true)),
	LogParams:    logimpl.ForOneShot("PROCESS", "trace", false),
}

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		fx.Supply(mockCoreBundleParams),
		fx.Supply(workloadmeta.NewParams()),
		fx.Provide(func() types.CheckComponent { return nil }),
		core.MockBundle(),
	)
}

func TestBundleOneShot(t *testing.T) {
	runCmd := func(r runner.Component) {
		checks := r.GetProvidedChecks()
		require.Len(t, checks, 7)

		var names []string
		for _, c := range checks {
			c := c.Object()
			names = append(names, c.Name())
		}
		require.ElementsMatch(t, []string{
			"process",
			"container",
			"rtcontainer",
			"process_events",
			"connections",
			"pod",
			"process_discovery",
		}, names)
	}

	err := fxutil.OneShot(runCmd,
		fx.Supply(
			fx.Annotate(t, fx.As(new(testing.TB))),

			mockCoreBundleParams,
		),
		core.MockBundle(),
		Bundle(),
	)
	require.NoError(t, err)
}
