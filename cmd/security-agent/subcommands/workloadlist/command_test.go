// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadlist

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestWorkloadListCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"workload-list"},
		workloadList,
		func() {},
	)
}

func TestWorkloadURL(t *testing.T) {
	cfg := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		// fx.Replace(config.MockParams{Overrides: overrides}),
	))

	expected := "https://localhost:5010/agent/workload-list?verbose=true"
	got, err := workloadURL(cfg, true)
	assert.NoError(t, err)
	assert.Equal(t, expected, got)

	cfg = fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"security_agent.cmd_port": 1234,
			"cmd_host":                "127.0.0.1",
		}}),
	))

	expected = "https://127.0.0.1:1234/agent/workload-list"
	got, err = workloadURL(cfg, false)
	assert.NoError(t, err)
	assert.Equal(t, expected, got)
}
