// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package agentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	configComp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/processcheck/processcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func TestProcessAgentComponentOnLinux(t *testing.T) {
	tests := []struct {
		name                 string
		agentFlavor          string
		checksEnabled        bool
		runInCoreAgentConfig bool
		expected             bool
	}{
		{
			name:                 "process-agent with checks enabled and run in core-agent mode disabled",
			agentFlavor:          flavor.ProcessAgent,
			checksEnabled:        true,
			runInCoreAgentConfig: false,
			expected:             true,
		},
		{
			name:                 "process-agent with checks disabled and run in core-agent mode disabled",
			agentFlavor:          flavor.ProcessAgent,
			checksEnabled:        false,
			runInCoreAgentConfig: false,
			expected:             false,
		},
		{
			name:                 "process-agent with checks enabled and run in core-agent mode enabled",
			agentFlavor:          flavor.ProcessAgent,
			checksEnabled:        true,
			runInCoreAgentConfig: true,
			expected:             false,
		},
		{
			name:                 "default agent with checks enabled and run in core-agent mode enabled",
			agentFlavor:          flavor.DefaultAgent,
			checksEnabled:        true,
			runInCoreAgentConfig: true,
			expected:             true,
		},
		{
			name:                 "default agent with checks disabled and run in core-agent mode enabled",
			agentFlavor:          flavor.DefaultAgent,
			checksEnabled:        false,
			runInCoreAgentConfig: true,
			expected:             false,
		},
		{
			name:                 "default agent with checks enabled and run in core-agent mode disabled",
			agentFlavor:          flavor.DefaultAgent,
			checksEnabled:        true,
			runInCoreAgentConfig: false,
			expected:             false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flavor.SetFlavor(tc.agentFlavor)

			opts := []fx.Option{
				runnerimpl.Module(),
				hostinfoimpl.MockModule(),
				submitterimpl.MockModule(),
				tagger.MockModule(),
				Module(),

				fx.Replace(configComp.MockParams{Overrides: map[string]interface{}{
					"process_config.run_in_core_agent.enabled": tc.runInCoreAgentConfig,
				}}),
			}

			if tc.checksEnabled {
				opts = append(opts, processcheckimpl.MockModule())
			}

			agt := fxutil.Test[optional.Option[agent.Component]](t, fx.Options(opts...))

			assert.Equal(t, tc.expected, agt.IsSet())
		})
	}
}
