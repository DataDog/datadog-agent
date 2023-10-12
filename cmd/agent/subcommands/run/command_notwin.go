// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package run implements 'agent run' (and deprecated 'agent start').
package run

import (
	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	// core components

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsdDebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"
	otelcollector "github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.uber.org/fx"
	// runtime init routines
)

func run(log log.Component,
	config config.Component,
	flare flare.Component,
	telemetry telemetry.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsdDebug.Component,
	forwarder defaultforwarder.Component,
	rcclient rcclient.Component,
	metadataRunner runner.Component,
	demux *aggregator.AgentDemultiplexer,
	sharedSerializer serializer.MetricSerializer,
	cliParams *cliParams,
	logsAgent util.Optional[logsAgent.Component],
	otelcollector otelcollector.Component,
	_ netflowServer.Component,
) error {
	// commonRun provides a mechanism to have the shared run function not require the unused components
	// (i.e. here `_ netflowServer`).  The run function can have different parameters on different platforms
	// based on platform-specific components.  commonRun is the shared initialization.
	return commonRun(log, config, flare, telemetry, sysprobeconfig, server, capture, serverDebug, forwarder, rcclient, metadataRunner, demux, sharedSerializer, cliParams, logsAgent, otelcollector)
}

func getPlatformModules() fx.Option {
	return fx.Options()
}
