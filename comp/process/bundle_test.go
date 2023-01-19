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

	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the process components, since this is not done
		// automatically.
		fx.Invoke(func(r runner.Component) {}),
		fx.Invoke(func(r submitter.Component) {}),
		Bundle))
}

func TestBundleOneShot(t *testing.T) {
	runCmd := func(r runner.Component) {
		checks := r.GetChecks()
		require.Len(t, checks, 2)

		names := []string{}
		for _, c := range checks {
			require.True(t, c.IsEnabled())
			names = append(names, c.Name())
		}
		require.ElementsMatch(t, []string{"process", "container"}, names)

		require.NoError(t, r.Run(context.TODO()))
	}

	err := fxutil.OneShot(runCmd,
		Bundle,
	)
	require.NoError(t, err)
}
