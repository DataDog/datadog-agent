// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestInstallCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "install", "foo==1.0", "-v"},
		install,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo==1.0"}, cliParams.args)
			require.Equal(t, 1, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}

func TestRemoveCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "remove", "foo"},
		remove,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo"}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}

func TestFreezeCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "freeze"},
		list,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}

func TestShowCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "show", "foo"},
		show,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo"}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}
