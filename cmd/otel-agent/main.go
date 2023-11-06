// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// otel-agent is a binary meant for testing the comp/otelcol package to ensure that it is reusable
// both as a binary and as a part of the core agent.
//
// This binary is not part of the Datadog Agent package at this point, nor is it meant to be used as such.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/logs"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/otelcol"
	"github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/fx"
)

const (
	loggerName = "OTELCOL"
)

var cfgPath = flag.String("config", "/opt/datadog-agent/etc/datadog.yaml", "agent config path")

func run(
	c collector.Component,
	demux *aggregator.AgentDemultiplexer,
	logsAgent optional.Option[logsAgent.Component], //nolint:revive // TODO fix unused-parameter
) error {
	// Setup stats telemetry handler
	if sender, err := demux.GetDefaultSender(); err == nil {
		// TODO: to be removed when default telemetry is enabled.
		telemetry.RegisterStatsSender(sender)
	}
	return c.Start()
}

func main() {
	flag.Parse()
	err := fxutil.OneShot(run,
		core.Bundle,
		forwarder.Bundle,
		otelcol.Bundle,
		logs.Bundle,
		fx.Supply(
			core.BundleParams{
				ConfigParams: config.NewAgentParamsWithSecrets(*cfgPath),
				LogParams:    corelog.ForOneShot(loggerName, "debug", true),
			},
		),
		fx.Provide(newForwarderParams),
		demultiplexer.Module,
		fx.Provide(newSerializer),
		fx.Provide(func(cfg config.Component) demultiplexer.Params {
			opts := aggregator.DefaultAgentDemultiplexerOptions()
			opts.EnableNoAggregationPipeline = cfg.GetBool("dogstatsd_no_aggregation_pipeline")
			opts.UseDogstatsdContextLimiter = true
			opts.DogstatsdMaxMetricsTags = cfg.GetInt("dogstatsd_max_metrics_tags")
			return demultiplexer.Params{Options: opts}
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	for range ch {
		break
	}
}

func newForwarderParams(config config.Component, log corelog.Component) defaultforwarder.Params {
	return defaultforwarder.NewParams(config, log)
}

func newSerializer(demux *aggregator.AgentDemultiplexer) serializer.MetricSerializer {
	return demux.Serializer()
}
