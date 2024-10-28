// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
)

func TestStatusCommand(t *testing.T) {
	cmd := statusCommand(&command.GlobalParams{})
	cmd.GroupID = ""
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{cmd},
		[]string{"status"},
		status,
		func() {})
}
