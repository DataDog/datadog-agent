// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunCommand(t *testing.T) {
	fxutil.TestRun(t, func() error {
		return runFxWrapper(&command.GlobalParams{})
	})
}
