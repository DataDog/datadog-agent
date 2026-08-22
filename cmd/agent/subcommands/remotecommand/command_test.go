// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotecommand

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunListCommandsFxDependencies(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		runListCommands(&cliParams{GlobalParams: &command.GlobalParams{}})
	})
}

func TestRunExecuteCommandFxDependencies(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		runExecuteCommand(&cliParams{GlobalParams: &command.GlobalParams{}}, "agent.status", nil)
	})
}
