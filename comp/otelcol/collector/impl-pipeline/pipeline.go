// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collector implements the collector component
package collector

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/datatype"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	otlpEnabled = "feature_otlp_enabled"
)

// Requires specifies a list of dependencies required for the collector
// to be instantiated.
type Requires struct {
	// Lc specifies the lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

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

	Tagger tagger.Component
}

// Provides specifics the types returned by the constructor
type Provides struct {
	compdef.Out

	Comp           collector.Component
	FlareProvider  flaretypes.Provider
	StatusProvider status.InformationProvider
}

type collectorImpl struct {
	col            *otlp.Pipeline
	config         config.Component
	log            corelog.Component
	serializer     serializer.MetricSerializer
	logsAgent      optional.Option[logsagentpipeline.Component]
	inventoryAgent inventoryagent.Component
	tagger         tagger.Component
}

func (c *collectorImpl) start(context.Context) error {
	on := otlp.IsEnabled(c.config)
	c.inventoryAgent.Set(otlpEnabled, on)
	if !on {
		return nil
	}
	var logch chan *message.Message
	if v, ok := c.logsAgent.Get(); ok {
		if provider := v.GetPipelineProvider(); provider != nil {
			logch = provider.NextPipelineChan()
		}
	}
	var err error
	col, err := otlp.NewPipelineFromAgentConfig(c.config, c.serializer, logch, c.tagger)
	if err != nil {
		// failure to start the OTLP component shouldn't fail startup
		c.log.Errorf("Error creating the OTLP ingest pipeline: %v", err)
		return nil
	}
	c.col = col
	// the context passed to this function has a startup deadline which
	// will shutdown the collector prematurely
	ctx := context.Background()
	go func() {
		if err := col.Run(ctx); err != nil {
			c.log.Errorf("Error running the OTLP ingest pipeline: %v", err)
		}
	}()
	return nil
}

func (c *collectorImpl) stop(context.Context) error {
	if c.col != nil {
		c.col.Stop()
	}
	return nil
}

// Status returns the status of the collector.
func (c *collectorImpl) Status() datatype.CollectorStatus {
	return c.col.GetCollectorStatus()
}

// NewComponent creates a new Component for this module and returns any errors on failure.
func NewComponent(reqs Requires) (Provides, error) {
	collector := &collectorImpl{
		config:         reqs.Config,
		log:            reqs.Log,
		serializer:     reqs.Serializer,
		logsAgent:      reqs.LogsAgent,
		inventoryAgent: reqs.InventoryAgent,
		tagger:         reqs.Tagger,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: collector.start,
		OnStop:  collector.stop,
	})

	return Provides{
		Comp:           collector,
		FlareProvider:  flaretypes.NewProvider(collector.fillFlare),
		StatusProvider: status.NewInformationProvider(collector),
	}, nil
}
