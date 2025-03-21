// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:generate mdatagen metadata.yaml

package datadogconnector // import "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/connector/datadogconnector"

import (
	"context"
	"fmt"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"time"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/featuregate"
)

type factory struct {
	tagger tagger.Component
}

const nativeIngestFeatureGateName = "connector.datadogconnector.NativeIngest"

// NativeIngestFeatureGate is the feature gate that controls native OTel spans ingestion in Datadog APM stats
var NativeIngestFeatureGate = featuregate.GlobalRegistry().MustRegister(
	nativeIngestFeatureGateName,
	featuregate.StageBeta,
	featuregate.WithRegisterDescription("When enabled, datadogconnector uses the native OTel API to ingest OTel spans and produce APM stats."),
	featuregate.WithRegisterFromVersion("v0.104.0"),
)

// NewFactoryForAgent ... XXX
func NewFactoryForAgent(tagger tagger.Component) connector.Factory {
	f := &factory{
		tagger: tagger,
	}

	//  OTel connector factory to make a factory for connectors
	return connector.NewFactory(
		Type,
		createDefaultConfig,
		connector.WithTracesToMetrics(f.createTracesToMetricsConnector, TracesToMetricsStability),
		connector.WithTracesToTraces(f.createTracesToTracesConnector, TracesToTracesStability))
}

// NewFactory creates a factory for datadog connector.
func NewFactory() connector.Factory {
	//  OTel connector factory to make a factory for connectors
	return NewFactoryForAgent(nil)
}

func createDefaultConfig() component.Config {
	return &Config{
		Traces: datadogconfig.TracesConnectorConfig{
			TracesConfig: datadogconfig.TracesConfig{
				IgnoreResources:        []string{},
				PeerServiceAggregation: true,
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
	metricsClient := metricsclient.InitializeMetricClient(params.MeterProvider, metricsclient.ConnectorSourceTag)

	fmt.Println("******** Inside createTracesToMetricsConnector ********")
	params.Logger.Info("******** Inside createTracesToMetricsConnector ********")

	c, err = newTraceToMetricConnector(params.TelemetrySettings, cfg, nextConsumer, metricsClient, f.tagger)

	if err != nil {
		return nil, err
	}
	return c, nil
}

func (f *factory) createTracesToTracesConnector(_ context.Context, params connector.Settings, _ component.Config, nextConsumer consumer.Traces) (connector.Traces, error) {
	return newTraceToTraceConnector(params.Logger, nextConsumer), nil
}
