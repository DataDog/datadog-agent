// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
)

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand("Agent"),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"version"},
		run,
		func() {})
}
