// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build e2ecoverage

package coverage

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCoverageGenerateCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(func() *subcommands.GlobalParams {
			return &subcommands.GlobalParams{}
		})},
		[]string{"coverage"},
		requestCoverage,
		func() {},
	)
}
