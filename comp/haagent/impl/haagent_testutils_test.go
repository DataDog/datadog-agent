// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newTestHaAgentComponent(t *testing.T, agentConfigs map[string]interface{}, logger log.Component) Provides {
	if logger == nil {
		logger = logmock.New(t)
	}
	agentConfigComponent := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: agentConfigs}),
	))

	requires := Requires{
		Logger:      logger,
		AgentConfig: agentConfigComponent,
		Hostname:    hostnameimpl.NewHostnameService(),
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)
	return provides
}
