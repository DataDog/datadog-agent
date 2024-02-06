// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package agentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"

	configComp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/processcheck/processcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkMocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func TestProcessAgentComponentOnLinux(t *testing.T) {
	tests := []struct {
		name                 string
		agentFlavor          string
		checksEnabled        bool
		checkName            string
		runInCoreAgentConfig bool
		expected             bool
	}{
		{
			name:                 "process-agent with process check enabled and run in core-agent mode disabled",
			agentFlavor:          flavor.ProcessAgent,
			checksEnabled:        true,
			checkName:            checks.ProcessCheckName,
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
			name:                 "process-agent with process check enabled and run in core-agent mode enabled",
			agentFlavor:          flavor.ProcessAgent,
			checksEnabled:        true,
			checkName:            checks.ProcessCheckName,
			runInCoreAgentConfig: true,
			expected:             false,
		},
		{
			name:                 "process-agent with connections check enabled and run in core-agent mode enabled",
			agentFlavor:          flavor.ProcessAgent,
			checksEnabled:        true,
			checkName:            checks.ConnectionsCheckName,
			runInCoreAgentConfig: true,
			expected:             true,
		},
		{
			name:                 "core agent with process check enabled and run in core-agent mode enabled",
			agentFlavor:          flavor.DefaultAgent,
			checksEnabled:        true,
			checkName:            checks.ProcessCheckName,
			runInCoreAgentConfig: true,
			expected:             true,
		},
		{
			name:                 "core agent with checks disabled and run in core-agent mode enabled",
			agentFlavor:          flavor.DefaultAgent,
			checksEnabled:        false,
			runInCoreAgentConfig: true,
			expected:             false,
		},
		{
			name:                 "core agent with process check enabled and run in core-agent mode disabled",
			agentFlavor:          flavor.DefaultAgent,
			checksEnabled:        true,
			checkName:            checks.ProcessCheckName,
			runInCoreAgentConfig: false,
			expected:             false,
		},
		{
			name:                 "core agent with connections enabled and run in core-agent mode enabled",
			agentFlavor:          flavor.DefaultAgent,
			checksEnabled:        true,
			checkName:            checks.ConnectionsCheckName,
			runInCoreAgentConfig: true,
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
				opts = append(opts, fx.Provide(func() func(c *checkMocks.Check) {
					return func(c *checkMocks.Check) {
						c.On("Init", mock.Anything, mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
						c.On("Name").Return(tc.checkName).Maybe()
						c.On("SupportsRunOptions").Return(false).Maybe()
						c.On("Realtime").Return(false).Maybe()
						c.On("Cleanup").Maybe()
						c.On("Run", mock.Anything, mock.Anything).Return(&checks.StandardRunResult{}, nil).Maybe()
						c.On("ShouldSaveLastRun").Return(false).Maybe()
						c.On("IsEnabled").Return(true).Maybe()
					}
				}))
			}

			agt := fxutil.Test[optional.Option[agent.Component]](t, fx.Options(opts...))

			assert.Equal(t, tc.expected, agt.IsSet())
		})
	}
}
