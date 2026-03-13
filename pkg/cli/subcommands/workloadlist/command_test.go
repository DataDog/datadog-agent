// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadlist

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"workload-list"},
		workloadList,
		func(cliParams *cliParams, _ core.BundleParams) {
			// Test default (non-JSON) output
			if cliParams.json {
				t.Errorf("expected json to be false by default")
			}
			if cliParams.prettyJSON {
				t.Errorf("expected prettyJSON to be false by default")
			}
			if cliParams.verboseList {
				t.Errorf("expected verboseList to be false by default")
			}
		})
}
