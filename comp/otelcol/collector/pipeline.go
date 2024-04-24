// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package collector

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/logsagentpipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	otlpEnabled = "feature_otlp_enabled"
)

// dependencies specifies a list of dependencies required for the collector
// to be instantiated.
type dependencies struct {
	fx.In

	// Lc specifies the fx lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc fx.Lifecycle

	// Config specifies the Datadog Agent's configuration component.
	Config config.Component

	// Log specifies the logging component.
	Log corelog.Component

	// Serializer specifies the metrics serializer that is used to export metrics
	// to Datadog.
	Serializer serializer.MetricSerializer

	// LogsAgent specifies a logs agent
	LogsAgent optional.Option[logsagentpipeline.Component]

	// InventoryAgent require the inventory metadata payload, allowing otelcol to add data to it.
	InventoryAgent inventoryagent.Component
}

type provides struct {
	fx.Out

	Comp           Component
	StatusProvider status.InformationProvider
}

type collector struct {
	deps dependencies
	col  *otlp.Pipeline
}

func (c *collector) Start() error {
	deps := c.deps
	on := otlp.IsEnabled(deps.Config)
	deps.InventoryAgent.Set(otlpEnabled, on)
	if !on {
		return nil
	}
	var logch chan *message.Message
	if v, ok := deps.LogsAgent.Get(); ok {
		if provider := v.GetPipelineProvider(); provider != nil {
			logch = provider.NextPipelineChan()
		}
	}
	var err error
	col, err := otlp.NewPipelineFromAgentConfig(deps.Config, deps.Serializer, logch)
	if err != nil {
		// failure to start the OTLP component shouldn't fail startup
		deps.Log.Errorf("Error creating the OTLP ingest pipeline: %v", err)
		return nil
	}
	c.col = col
	// the context passed to this function has a startup deadline which
	// will shutdown the collector prematurely
	ctx := context.Background()
	go func() {
		if err := col.Run(ctx); err != nil {
			deps.Log.Errorf("Error running the OTLP ingest pipeline: %v", err)
		}
	}()
	return nil
}

func (c *collector) Stop() {
	if c.col != nil {
		c.col.Stop()
	}
}

func (c *collector) stop(context.Context) error {
	c.Stop()
	return nil
}

func (c *collector) start(context.Context) error {
	return c.Start()
}

// Status returns the status of the collector.
func (c *collector) Status() otlp.CollectorStatus {
	return c.col.GetCollectorStatus()
}

// newPipeline creates a new Component for this module and returns any errors on failure.
func newPipeline(deps dependencies) (provides, error) {
	collector := &collector{
		deps: deps,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: collector.start,
		OnStop:  collector.stop,
	})

	return provides{
		Comp:           collector,
		StatusProvider: status.NewInformationProvider(collector),
	}, nil
}
