// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestInstallCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		apiCommands(&command.GlobalParams{}),
		[]string{"install", "test", "v1"},
		install,
		func() {})
}

func TestStartExperimentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		apiCommands(&command.GlobalParams{}),
		[]string{"start-experiment", "test", "v1"},
		start,
		func() {})
}

func TestStopExperimentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		apiCommands(&command.GlobalParams{}),
		[]string{"stop-experiment", "test"},
		stop,
		func() {})
}

func TestPromoteExperimentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		apiCommands(&command.GlobalParams{}),
		[]string{"promote-experiment", "test"},
		promote,
		func() {})
}
