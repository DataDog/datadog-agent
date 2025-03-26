// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package policy holds policy CLI subcommand related files
package policy

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

func TestReloadRuntimePoliciesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{ReloadPoliciesCommand(&command.GlobalParams{})},
		[]string{"runtime", "policy", "reload"},
		reloadRuntimePolicies,
		func() {})
}
