// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dumpconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

func TestRun(t *testing.T) {
	for _, target := range []string{"core", "system-probe"} {
		t.Run(target, func(t *testing.T) {
			require.NoError(t, run(&cliParams{GlobalParams: &command.GlobalParams{}, Target: target}))
		})
	}
}

func TestRunUnknownTarget(t *testing.T) {
	err := run(&cliParams{GlobalParams: &command.GlobalParams{}, Target: "unknown"})
	assert.ErrorContains(t, err, "unknown target")
}
