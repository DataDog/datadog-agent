// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !e2ecoverage

// Package coverage does nothing when compiling without the e2ecoverage build tag.
package coverage

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
)

// SetupCoverageCommand does nothing when compiling without the e2ecoverage build tag
func SetupCoverageCommand(_ func() *subcommands.GlobalParams, _ *cobra.Command) {}
