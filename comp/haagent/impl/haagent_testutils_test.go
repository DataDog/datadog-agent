// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type testDeps struct {
	fx.In

	Config         config.Component
	Log            log.Component
	InventoryAgent inventoryagent.Component
}

func newTestHaAgentComponent(t *testing.T, agentConfigs map[string]interface{}) (Provides, testDeps) {
	deps := fxutil.Test[testDeps](t, fx.Options(
		fx.Supply(configComponent.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		configComponent.MockModule(),
		hostnameimpl.MockModule(),
		fx.Replace(configComponent.MockParams{Overrides: agentConfigs}),
		inventoryagentimpl.MockModule(),
	))

	requires := Requires{
		Logger:         deps.Log,
		AgentConfig:    deps.Config,
		InventoryAgent: deps.InventoryAgent,
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)
	return provides, deps
}
