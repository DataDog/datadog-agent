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
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var mockCoreBundleParams = core.BundleParams{
	ConfigParams: configComp.NewParams("", configComp.WithConfigMissingOK(true)),
	LogParams:    log.LogForOneShot("PROCESS", "trace", false),
}

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		fx.Supply(
			fx.Annotate(t, fx.As(new(testing.TB))),

			mockCoreBundleParams,
		),

		utils.DisableContainerFeatures,

		// Start the runner
		fx.Invoke(func(r runner.Component) {}),

		Bundle,
	))
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

		utils.DisableContainerFeatures,

		Bundle,
	)
	require.NoError(t, err)
}
