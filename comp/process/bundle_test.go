// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var testHostInfo = &checks.HostInfo{SystemInfo: &model.SystemInfo{}}

func TestBundleDependencies(t *testing.T) {
	// Don't enable any features, as the container check won't work in all environments
	config.SetDetectedFeatures(config.FeatureMap{})
	t.Cleanup(func() { config.SetDetectedFeatures(nil) })

	require.NoError(t, fx.ValidateApp(
		fx.Supply(
			fx.Annotate(t, fx.As(new(testing.TB))),

			testHostInfo,
			&sysconfig.Config{},
		),

		// instantiate all of the process components, since this is not done
		// automatically.
		fx.Invoke(func(r runner.Component) {}),

		Bundle))
}

func TestBundleOneShot(t *testing.T) {
	// Don't enable any features, we haven't set up a container provider so the container check will crash
	config.SetDetectedFeatures(config.FeatureMap{})
	t.Cleanup(func() { config.SetDetectedFeatures(nil) })

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

			testHostInfo,
			&sysconfig.Config{},
		),

		Bundle,
	)
	require.NoError(t, err)
}
