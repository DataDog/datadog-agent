// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build hostprofiler

package run

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestFxRun tests that fx can build dependencies for the run command.
func TestFxRun(t *testing.T) {
	fxutil.TestRun(t, func() error {
		params := &cliParams{
			GlobalParams: &subcommands.GlobalParams{},
		}
		return runHostProfilerCommand(context.Background(), params)
	})
}
