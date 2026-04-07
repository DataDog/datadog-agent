// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package controlsvc

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/windows/controlsvc"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartServiceCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"start-service"},
		controlsvc.StartService,
		func() {})
}

func TestStopServiceCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"stopservice"},
		controlsvc.StopService,
		func() {})
}

func TestRestartServiceCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"restart-service"},
		controlsvc.RestartService,
		func() {})
}
