// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
)

const (
	// TypeStr defines the serializer exporter type string.
	TypeStr   = "serializer"
	stability = component.StabilityLevelStable
)

type factory struct {
	s              serializer.MetricSerializer
	enricher       tagenricher
	hostGetter     SourceProviderFunc
	statsIn        chan []byte
	wg             *sync.WaitGroup // waits for consumeStatsPayload to exit
	createConsumer createConsumerFunc
}

type tagenricher interface {
	SetCardinality(cardinality string) error
	Enrich(ctx context.Context, extraTags []string, dimensions *otlpmetrics.Dimensions) []string
}

type defaultTagEnricher struct{}

func (d *defaultTagEnricher) SetCardinality(_ string) error {
	return nil
}

func (d *defaultTagEnricher) Enrich(_ context.Context, extraTags []string, dimensions *otlpmetrics.Dimensions) []string {
	enrichedTags := make([]string, 0, len(extraTags)+len(dimensions.Tags()))
	enrichedTags = append(enrichedTags, extraTags...)
	enrichedTags = append(enrichedTags, dimensions.Tags()...)
	return enrichedTags
}

type createConsumerFunc func(enricher tagenricher, extraTags []string, apmReceiverAddr string, buildInfo component.BuildInfo) SerializeConsumer

// NewFactoryForAgent creates a new serializer exporter factory.
func NewFactoryForAgent(s serializer.MetricSerializer, enricher tagenricher, hostGetter func(context.Context) (string, error), statsIn chan []byte, wg *sync.WaitGroup) exp.Factory {
	f := &factory{
		s:          s,
		enricher:   enricher,
		hostGetter: hostGetter,
		statsIn:    statsIn,
		wg:         wg,
		createConsumer: func(enricher tagenricher, extraTags []string, apmReceiverAddr string, _ component.BuildInfo) SerializeConsumer {
			return &serializerConsumer{enricher: enricher, extraTags: extraTags, apmReceiverAddr: apmReceiverAddr}
		},
	}
	cfgType, _ := component.NewType(TypeStr)

	return exp.NewFactory(
		cfgType,
		newDefaultConfig,
		exp.WithMetrics(f.createMetricExporter, stability),
	)
}

// NewFactory creates a new factory for the serializer exporter.
func NewFactory() exp.Factory {
	f := &factory{
		enricher: &defaultTagEnricher{},
		// send empty hostname to the serializer
		hostGetter: func(context.Context) (string, error) {
			return "new-otel-test-host", nil
		},
		createConsumer: func(enricher tagenricher, extraTags []string, apmReceiverAddr string, buildInfo component.BuildInfo) SerializeConsumer {
			s := &serializerConsumer{enricher: enricher, extraTags: extraTags, apmReceiverAddr: apmReceiverAddr}
			return &collectorConsumer{
				serializerConsumer: s,
				seenHosts:          make(map[string]struct{}),
				seenTags:           make(map[string]struct{}),
				buildInfo:          buildInfo,
				gatewayUsage:       attributes.NewGatewayUsage(),
				getPushTime:        func() uint64 { return uint64(time.Now().Unix()) },
			}
		},
	}
	cfgType, _ := component.NewType(TypeStr)
	return exp.NewFactory(
		cfgType,
		newDefaultConfig,
		exp.WithMetrics(f.createMetricExporter, stability),
	)
}

// createMetricsExporter creates a new metrics exporter.
func (f *factory) createMetricExporter(ctx context.Context, params exp.Settings, c component.Config) (exp.Metrics, error) {
	var err error
	cfg := c.(*ExporterConfig)
	var forwader *defaultforwarder.DefaultForwarder
	if f.s == nil {
		f.s, forwader, err = initSerializer(params.Logger, cfg, f.hostGetter)
		if err != nil {
			return nil, err
		}
		go func() {
			params.Logger.Info("starting forwarder")
			err := forwader.Start()
			if err != nil {
				params.Logger.Error("failed to start forwarder", zap.Error(err))
			}
		}()

	}

	// TODO: Ideally the attributes translator would be created once and reused
	// across all signals. This would need unifying the logsagent and serializer
	// exporters into a single exporter.
	attributesTranslator, err := attributes.NewTranslator(params.TelemetrySettings)
	if err != nil {
		return nil, err
	}

	newExp, err := NewExporter(params, attributesTranslator, f.s, cfg, f.enricher, f.hostGetter, f.statsIn, f.createConsumer)
	if err != nil {
		return nil, err
	}

	exporter, err := exporterhelper.NewMetrics(ctx, params, cfg, newExp.ConsumeMetrics,
		exporterhelper.WithQueue(cfg.QueueConfig),
		exporterhelper.WithTimeout(cfg.TimeoutConfig),
		// the metrics remapping code mutates data
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
		exporterhelper.WithShutdown(func(context.Context) error {
			if f.wg != nil {
				f.wg.Wait() // wait for consumeStatsPayload to exit
			}
			if f.statsIn != nil {
				close(f.statsIn)
			}
			if forwader != nil {
				forwader.Stop()
			}
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	return resourcetotelemetry.WrapMetricsExporter(
		resourcetotelemetry.Settings{Enabled: cfg.Metrics.Metrics.ExporterConfig.ResourceAttributesAsTags}, exporter), nil
}
