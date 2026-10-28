// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package dcaconfigcheck

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"configcheck"},
		run,
		func() {})
}

func TestLogLevelIgnoresEnv(t *testing.T) {
	t.Setenv("DD_LOG_LEVEL", "info")

	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(func() GlobalParams { return GlobalParams{} })},
		[]string{"configcheck"},
		run,
		func(bundleParams core.BundleParams) {
			assert.Equal(t, "off", bundleParams.LogParams.LogLevelFn(nil))
		})

	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(func() GlobalParams { return GlobalParams{} })},
		[]string{"configcheck", "--log_level", "debug"},
		run,
		func(bundleParams core.BundleParams) {
			assert.Equal(t, "debug", bundleParams.LogParams.LogLevelFn(nil))
		})
}
