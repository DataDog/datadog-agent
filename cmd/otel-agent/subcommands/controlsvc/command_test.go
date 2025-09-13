// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && otlp && test

package controlsvc

import (
	"testing"

	winctrl "github.com/DataDog/datadog-agent/cmd/otel-agent/windows/controlsvc"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestControlSvcOneShotSubcommands(t *testing.T) {
	cmds := Commands(nil)

	// stop-service → OneShot(winctrl.StopService)
	fxutil.TestOneShotSubcommand(
		t,
		cmds,
		[]string{"stop-service"},
		winctrl.StopService,
		func() {},
	)

	// restart-service → OneShot(winctrl.RestartService)
	fxutil.TestOneShotSubcommand(
		t,
		cmds,
		[]string{"restart-service"},
		winctrl.RestartService,
		func() {},
	)
}
