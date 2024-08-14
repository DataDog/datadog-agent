// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
)

func TestRunCommand(t *testing.T) {
	global := &command.GlobalParams{}
	// Validates that our dependency graph is valid.
	// This does not instantiate any component and merely validates the
	// dependency graph.
	require.NoError(t, fx.ValidateApp(getFxOptions(global)...))
}

func TestAppStartsAndStops(t *testing.T) {
	global := &command.GlobalParams{}
	// Creates a new test app (not a daemon.windowsService)
	// with our dependency graph and verify that we can start and stop it.
	// This is essentially what the svc.Run code does behind the scenes.
	// Note: this actually instantiates the components, so it will actually start
	// the remote config service etc...
	testApp := fxtest.New(t, getFxOptions(global)...)
	testApp.RequireStart()
	testApp.RequireStop()
}
