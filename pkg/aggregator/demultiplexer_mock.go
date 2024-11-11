// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// TestDeps contains dependencies for InitAndStartAgentDemultiplexerForTest
type TestDeps struct {
	fx.In
	Log             log.Component
	Hostname        hostname.Component
	SharedForwarder defaultforwarder.Component
	Compressor      compression.Component
}

// InitAndStartAgentDemultiplexerForTest initializes an aggregator for tests.
func InitAndStartAgentDemultiplexerForTest(deps TestDeps, options AgentDemultiplexerOptions, hostname string) *AgentDemultiplexer {
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	eventPlatformForwarder := optional.NewOptionPtr[eventplatform.Forwarder](eventplatformimpl.NewNoopEventPlatformForwarder(deps.Hostname))
	return InitAndStartAgentDemultiplexer(deps.Log, deps.SharedForwarder, &orchestratorForwarder, options, eventPlatformForwarder, deps.Compressor, nooptagger.NewTaggerClient(), hostname)
}
