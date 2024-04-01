// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestFxRun(t *testing.T) {
	fxutil.TestRun(t, func() error {
		ctx := context.Background()
		cliParams := &subcommands.GlobalParams{}
		return runOTelAgentCommand(ctx, cliParams)
	})
}
