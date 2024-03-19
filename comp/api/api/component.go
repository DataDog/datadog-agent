// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package api implements the internal Agent API which exposes endpoints such as config, flare or status
package api

import (
	"net"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// team: agent-shared-components

// TODO(components):
// * Lifecycle can't be used atm because:
//     - logsAgent and remoteconfig.Service are modified in `startAgent` in the run subcommand
//     - Same for workloadmeta and senderManager in `execJmxCommand` in the jmx subcommand

// Component is the component type.
type Component interface {
	StartServer(
		wmeta workloadmeta.Component,
		tagger tagger.Component,
		ac autodiscovery.Component,
		logsAgent optional.Option[logsAgent.Component],
		senderManager sender.DiagnoseSenderManager,
		collector optional.Option[collector.Component],
	) error
	StopServer()
	ServerAddress() *net.TCPAddr
}
