// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"
)

// TestDeps contains dependencies for InitAndStartAgentDemultiplexerForTest
type TestDeps struct {
	fx.In
	Log             log.Component
	SharedForwarder defaultforwarder.Component
}

// InitAndStartAgentDemultiplexerForTest initializes an aggregator for tests.
func InitAndStartAgentDemultiplexerForTest(deps TestDeps, options AgentDemultiplexerOptions, hostname string) *AgentDemultiplexer {
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	return InitAndStartAgentDemultiplexer(deps.Log, deps.SharedForwarder, &orchestratorForwarder, options, hostname)
}
