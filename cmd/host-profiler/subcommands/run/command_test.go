// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package run

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configsyncimpl "github.com/DataDog/datadog-agent/comp/core/configsync/impl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestFxRun tests that fx can build dependencies for the run command.
func TestFxRunWithoutAgentCore(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		MakeCommand(func() *globalparams.GlobalParams {
			return &globalparams.GlobalParams{ConfFilePath: "config_path"}
		}),
		[]string{"run"},
		run,
		func() {})
}

func TestFxRunWithAgentCore(t *testing.T) {
	// Use fxutil.TestOneShot as TestOneShotSubcommand would require valid datadog.yaml file, auth_token file and ipc_cert.pem.
	fxutil.TestOneShot(t, func() {
		runHostProfilerCommand(context.Background(), &cliParams{GlobalParams: &globalparams.GlobalParams{CoreConfPath: "config_path"}})
	})
}

type fakeConfigStream bool

func (f fakeConfigStream) IsActive() bool { return bool(f) }

func TestConfigSyncFallback(t *testing.T) {
	deps := configsyncimpl.Requires{Config: config.NewMock(t), Log: logmock.New(t)}

	component, err := newConfigSyncFallback(deps, fakeConfigStream(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := component.(noopConfigSync); !ok {
		t.Fatal("expected configsync to be skipped when configstream is active")
	}

	component, err = newConfigSyncFallback(deps, fakeConfigStream(false))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := component.(noopConfigSync); ok {
		t.Fatal("expected configsync to be created when configstream is inactive")
	}
}
