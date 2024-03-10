// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

package agentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/processcheck/processcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestProcessAgentComponent(t *testing.T) {
	tests := []struct {
		name          string
		agentFlavor   string
		checksEnabled bool
		expected      bool
	}{
		{
			name:          "process agent and checks enabled",
			agentFlavor:   flavor.ProcessAgent,
			checksEnabled: true,
			expected:      true,
		},
		{
			name:          "process agent and checks disabled",
			agentFlavor:   flavor.ProcessAgent,
			checksEnabled: false,
			expected:      false,
		},
		{
			name:          "default agent and checks enabled",
			agentFlavor:   flavor.DefaultAgent,
			checksEnabled: true,
			expected:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			originalFlavor := flavor.GetFlavor()
			flavor.SetFlavor(tc.agentFlavor)
			defer func() {
				flavor.SetFlavor(originalFlavor)
			}()

			opts := []fx.Option{
				runnerimpl.Module(),
				hostinfoimpl.MockModule(),
				submitterimpl.MockModule(),
				tagger.MockModule(),
				Module(),
			}

			if tc.checksEnabled {
				opts = append(opts, processcheckimpl.MockModule())
			}

			agentComponent := fxutil.Test[agent.Component](t, fx.Options(opts...))
			assert.Equal(t, tc.expected, agentComponent.Enabled())
		})
	}
}
