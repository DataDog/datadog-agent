// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package run

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func TestFxRun(t *testing.T) {
	fxutil.TestRun(t, func() error {
		ctx := context.Background()
		cliParams := &subcommands.GlobalParams{}
		return runOTelAgentCommand(ctx, cliParams, fx.Provide(compdef.NewTestLifecycle), fx.Provide(func(lc *compdef.TestLifecycle) compdef.Lifecycle {
			return lc
		}),
		)
	})
}
