// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogconnector

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"

	datadogconfig "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"
)

type factory struct {
	tagger       types.TaggerClient
	concentrator *stats.Concentrator
	hostname     option.Option[string]
	// agentConfig is the configuration of the trace-agent behind the datadog exporter
	agentConfig *config.AgentConfig
}

// SourceProviderFunc is a function that returns the source of the host.
type SourceProviderFunc func(context.Context) (string, error)

// FactoryOption configures the datadog connector factory.
type FactoryOption func(*factory)

// WithAgentConfig excludes from APM stats the traces whose root span the trace-agent behind the datadog exporter
// drops: its root resource matches apm_config.ignore_resources, or it fails apm_config.filter_tags or
// apm_config.filter_tags_regex. conf is the trace-agent's configuration; it is only read.
func WithAgentConfig(conf *config.AgentConfig) FactoryOption {
	return func(f *factory) {
		f.agentConfig = conf
	}
}

// NewConnectorFactory creates a factory for datadog connector for use in OTel agent
func NewConnectorFactory(componentType component.Type, metricsStability, traceStability component.StabilityLevel, tagger types.TaggerClient, hostGetter SourceProviderFunc, concentrator *stats.Concentrator, opts ...FactoryOption) connector.Factory {
	f := &factory{
		tagger:       tagger,
		concentrator: concentrator,
	}
	for _, opt := range opts {
		opt(f)
	}

	if hostGetter != nil {
		if hostname, err := hostGetter(context.Background()); err == nil {
			f.hostname = option.New(hostname)
		}
	}

	//  OTel connector factory to make a factory for connectors
	return connector.NewFactory(
		componentType,
		createDefaultConfig,
		connector.WithTracesToMetrics(f.createTracesToMetricsConnector, metricsStability),
		connector.WithTracesToTraces(createTracesToTracesConnector, traceStability))
}

func createDefaultConfig() component.Config {
	return &datadogconfig.ConnectorComponentConfig{
		Traces: datadogconfig.TracesConnectorConfig{
			TracesConfig: datadogconfig.TracesConfig{
				IgnoreResources:        []string{},
				PeerTagsAggregation:    true,
				ComputeStatsBySpanKind: true,
			},

			TraceBuffer:    1000,
			BucketInterval: 10 * time.Second,
		},
	}
}

// defines the consumer type of the connector
// we want to consume traces and export metrics therefore define nextConsumer as metrics, consumer is the next component in the pipeline
func (f *factory) createTracesToMetricsConnector(_ context.Context, params connector.Settings, cfg component.Config, nextConsumer consumer.Metrics) (c connector.Traces, err error) {
	metricsClient, err := metricsclient.InitializeMetricClient(params.MeterProvider, metricsclient.ConnectorSourceTag)
	if err != nil {
		return nil, err
	}
	conn, err := newTraceToMetricConnector(params.TelemetrySettings, cfg, nextConsumer, metricsClient, f.concentrator, f.tagger, f.hostname)
	if err != nil {
		return nil, err
	}
	conn.rootFilter = newRootFilter(f.agentConfig)
	return conn, nil
}

func createTracesToTracesConnector(_ context.Context, params connector.Settings, _ component.Config, nextConsumer consumer.Traces) (connector.Traces, error) {
	return newTraceToTraceConnector(params.Logger, nextConsumer), nil
}
