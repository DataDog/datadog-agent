package contrib

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type dependencies struct {
	fx.In
	// Lc specifies the fx lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc fx.Lifecycle

	// Log specifies the logging component.
	Log              corelog.Component
	Provider         otelcol.ConfigProvider
	CollectorContrib collectorcontrib.Component
	Serializer       serializer.MetricSerializer
	LogsAgent        optional.Option[logsagentpipeline.Component]
	HostName         hostname.Component
}

type collector struct {
	deps dependencies
	set  otelcol.CollectorSettings
	col  *otelcol.Collector
}

func NewCollector(deps dependencies) (def.Component, error) {
	fmt.Printf("##### NewCollector\n")
	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Version: "1.0.0",
		},
		Factories: func() (otelcol.Factories, error) {
			factories, err := deps.CollectorContrib.OTelComponentFactories()
			if err != nil {
				return otelcol.Factories{}, err
			}
			if v, ok := deps.LogsAgent.Get(); ok {
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(deps.Serializer, v, deps.HostName)
			} else {
				fmt.Printf("##### LogsAgent not found\n")
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(deps.Serializer, nil, deps.HostName)
			}
			fmt.Printf("##### Factories: %#v\n", factories)
			return factories, nil
		},
		ConfigProvider: deps.Provider,
	}
	col, err := otelcol.NewCollector(set)
	if err != nil {
		return nil, err
	}
	c := &collector{
		deps: deps,
		set:  set,
		col:  col,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: c.Start,
		OnStop:  c.Stop,
	})
	return c, nil
}

func (c *collector) Start(context.Context) error {
	go func() {
		if err := c.col.Run(context.Background()); err != nil {
			c.deps.Log.Errorf("Error running the OTLP ingest pipeline: %v", err)
		}
	}()
	return nil
}

func (c *collector) Stop(ctx context.Context) error {
	c.col.Shutdown()
	return nil
}

func (c *collector) Status() otlp.CollectorStatus {
	return otlp.CollectorStatus{
		Status: c.col.GetState().String(),
	}
}
