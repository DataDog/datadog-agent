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

	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	corelogimpl "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	"github.com/DataDog/datadog-agent/comp/logs"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol"
	"github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/pkg/serializer"
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
	forwarder defaultforwarder.Component,
	logsAgent optional.Option[logsAgent.Component], //nolint:revive // TODO fix unused-parameter
) error {
	// Setup stats telemetry handler
	return forwarder.Start()
}

type orchestratorinterfaceimpl struct {
	f defaultforwarder.Forwarder
}

func NewOrchestratorinterfaceimpl(f defaultforwarder.Forwarder) orchestratorinterface.Component {
	return &orchestratorinterfaceimpl{
		f: f,
	}
}

func (o *orchestratorinterfaceimpl) Get() (defaultforwarder.Forwarder, bool) {
	return o.f, true
}

func (o *orchestratorinterfaceimpl) Reset() {
	o.f = nil
}

func main() {
	flag.Parse()
	err := fxutil.OneShot(run,
		forwarder.Bundle(),
		otelcol.Bundle(),
		config.Module(),
		corelogimpl.Module(),
		inventoryagentimpl.Module(),
		workloadmeta.Module(),
		hostnameimpl.Module(),
		sysprobeconfig.NoneModule(),
		fetchonlyimpl.Module(),
		fx.Provide(func() workloadmeta.Params {
			return workloadmeta.NewParams()
		}),

		fx.Provide(func() config.Params {
			return config.NewAgentParams(*cfgPath)
		}),
		fx.Provide(func() corelogimpl.Params {
			return corelogimpl.ForOneShot(loggerName, "debug", true)
		}),
		logs.Bundle(),
		fx.Provide(serializer.NewSerializer),
		fx.Provide(func(s *serializer.Serializer) serializer.MetricSerializer {
			return s
		}),
		fx.Provide(func() string {
			// TODO: send hostname
			return ""
		}),

		fx.Provide(newForwarderParams),
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		fx.Provide(NewOrchestratorinterfaceimpl),
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
