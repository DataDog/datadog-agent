// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package collector

import (
	"context"
	"embed"
	"io"
	"path"

	textTemplate "text/template"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	logsagent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
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
	LogsAgent optional.Option[logsagent.Component]

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

// Status returns the status of the collector.
func (c *collector) Status() otlp.CollectorStatus {
	return c.col.GetCollectorStatus()
}

// newPipeline creates a new Component for this module and returns any errors on failure.
func newPipeline(deps dependencies) (provides, error) {
	collector := &collector{deps: deps}

	return provides{
		Comp:           collector,
		StatusProvider: status.NewInformationProvider(collector),
	}, nil
}

//go:embed status_templates
var templatesFS embed.FS

func (c *collector) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	c.populateStatus(stats)

	return stats
}

func (c *collector) populateStatus(stats map[string]interface{}) {
	otlpStatus := make(map[string]interface{})
	otlpIsEnabled := otlp.IsEnabled(c.deps.Config)

	var otlpCollectorStatus otlp.CollectorStatus

	if otlpIsEnabled {
		otlpCollectorStatus = c.Status()
	} else {
		otlpCollectorStatus = otlp.CollectorStatus{Status: "Not running", ErrorMessage: ""}
	}
	otlpStatus["otlpStatus"] = otlpIsEnabled
	otlpStatus["otlpCollectorStatus"] = otlpCollectorStatus.Status
	otlpStatus["otlpCollectorStatusErr"] = otlpCollectorStatus.ErrorMessage

	stats["otlp"] = otlpStatus
}

func (c *collector) Name() string {
	return "OTLP"
}

func (c *collector) Section() string {
	return "OTLP"
}

func (c *collector) JSON(_ bool, stats map[string]interface{}) error {
	c.populateStatus(stats)

	return nil
}

func (c *collector) Text(_ bool, buffer io.Writer) error {
	return renderText(buffer, c.getStatusInfo())
}

func (c *collector) HTML(_ bool, _ io.Writer) error {
	return nil
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "otlp.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("otlp").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
