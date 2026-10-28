// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"health"},
		requestHealth,
		func(_ core.BundleParams) {})
}

func TestLogLevelIgnoresEnv(t *testing.T) {
	t.Setenv("DD_LOG_LEVEL", "info")

	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(func() GlobalParams { return GlobalParams{} })},
		[]string{"health"},
		requestHealth,
		func(bundleParams core.BundleParams) {
			assert.Equal(t, "off", bundleParams.LogParams.LogLevelFn(nil))
		})

	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(func() GlobalParams { return GlobalParams{} })},
		[]string{"health", "--log_level", "debug"},
		requestHealth,
		func(bundleParams core.BundleParams) {
			assert.Equal(t, "debug", bundleParams.LogParams.LogLevelFn(nil))
		})
}
