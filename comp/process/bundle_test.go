// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	configComp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/process/runner"

	coreStatusImpl "github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/process/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
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
		workloadmeta.Module(),
		coreStatusImpl.Module(),
		statusimpl.Module(),
		fx.Supply(tagger.NewFakeTaggerParams()),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: python.GetPythonVersion,
			},
		),
		fx.Provide(func() context.Context { return context.TODO() }),
	)
}

func TestBundleOneShot(t *testing.T) {
	runCmd := func(r runner.Component) {
		checks := r.GetProvidedChecks()
		require.Len(t, checks, 6)

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
			"process_discovery",
		}, names)
	}

	err := fxutil.OneShot(runCmd,
		fx.Supply(
			fx.Annotate(t, fx.As(new(testing.TB))),

			mockCoreBundleParams,
		),
		// sets a static hostname to avoid grpc call to get hostname from core-agent
		fx.Replace(configComp.MockParams{Overrides: map[string]interface{}{
			"hostname": "testhost",
		}}),
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.Module(),
		Bundle(),
	)
	require.NoError(t, err)
}
