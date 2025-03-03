// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata"
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
	pkgdatadog "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"
)

const (
	// TypeStr defines the serializer exporter type string.
	TypeStr   = "serializer"
	stability = component.StabilityLevelStable
)

type factory struct {
	s            serializer.MetricSerializer
	enricher     tagenricher
	hostProvider SourceProviderFunc

	statsIn chan []byte
	wg      *sync.WaitGroup // waits for consumeStatsPayload to exit

	createConsumer createConsumerFunc
	options        []otlpmetrics.TranslatorOption

	onceReporter sync.Once
	reporter     *inframetadata.Reporter
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

type createConsumerFunc func(enricher tagenricher, extraTags []string, apmReceiverAddr string, buildInfo component.BuildInfo) SerializerConsumer

// NewFactoryForAgent creates a new serializer exporter factory for Agent OTLP ingestion and embedded collector.
func NewFactoryForAgent(s serializer.MetricSerializer, enricher tagenricher, hostGetter SourceProviderFunc, statsIn chan []byte, wg *sync.WaitGroup) exp.Factory {
	var options []otlpmetrics.TranslatorOption
	if !pkgdatadog.MetricRemappingDisabledFeatureGate.IsEnabled() {
		options = append(options, otlpmetrics.WithOTelPrefix())
	}

	f := &factory{
		s:            s,
		enricher:     enricher,
		hostProvider: hostGetter,
		statsIn:      statsIn,
		wg:           wg,
		createConsumer: func(enricher tagenricher, extraTags []string, apmReceiverAddr string, _ component.BuildInfo) SerializerConsumer {
			return &serializerConsumer{enricher: enricher, extraTags: extraTags, apmReceiverAddr: apmReceiverAddr}
		},
		options: options,
	}
	cfgType := component.MustNewType(TypeStr)

	return exp.NewFactory(
		cfgType,
		newDefaultConfigForAgent,
		exp.WithMetrics(f.createMetricExporter, stability),
	)
}

// NewFactory creates a new factory for the serializer exporter.
func NewFactory() exp.Factory {
	var options []otlpmetrics.TranslatorOption
	if !pkgdatadog.MetricRemappingDisabledFeatureGate.IsEnabled() {
		options = append(options, otlpmetrics.WithRemapping())
	}

	f := &factory{
		enricher: &defaultTagEnricher{},
		// hostProvider is a no-op function that returns an empty host.
		// In OSS collector, the host is overridden via the HostProvider field in the config.
		hostProvider: func(_ context.Context) (string, error) { return "", nil },
		createConsumer: func(enricher tagenricher, extraTags []string, apmReceiverAddr string, buildInfo component.BuildInfo) SerializerConsumer {
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
		options: options,
	}
	cfgType := component.MustNewType(TypeStr)
	return exp.NewFactory(
		cfgType,
		newDefaultConfig,
		exp.WithMetrics(f.createMetricExporter, stability),
	)
}

// Reporter builds and returns an *inframetadata.Reporter.
func (f *factory) Reporter(params exp.Settings, forwarder defaultforwarder.Forwarder, reporterPeriod time.Duration) (*inframetadata.Reporter, error) {
	var reporterErr error
	f.onceReporter.Do(func() {
		pusher := &hostMetadataPusher{forwarder: forwarder}
		f.reporter, reporterErr = inframetadata.NewReporter(params.Logger, pusher, reporterPeriod)
		if reporterErr == nil {
			params.Logger.Info("Starting host metadata reporter")
			go func() {
				if err := f.reporter.Run(context.Background()); err != nil {
					params.Logger.Error("Host metadata reporter failed at runtime", zap.Error(err))
				}
			}()
		}
	})
	return f.reporter, reporterErr
}

// createMetricsExporter creates a new metrics exporter.
func (f *factory) createMetricExporter(ctx context.Context, params exp.Settings, c component.Config) (exp.Metrics, error) {
	var err error
	cfg := c.(*ExporterConfig)
	var forwarder *defaultforwarder.DefaultForwarder
	if f.s == nil {
		f.s, forwarder, err = initSerializer(params.Logger, cfg, f.hostProvider)
		if err != nil {
			return nil, err
		}
		go func() {
			params.Logger.Info("starting forwarder")
			err := forwarder.Start()
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
	hostGetter := f.hostProvider
	if cfg.HostProvider != nil {
		hostGetter = cfg.HostProvider
	}
	// Create the metrics translator
	tr, err := translatorFromConfig(params.TelemetrySettings, attributesTranslator, cfg.Metrics.Metrics, hostGetter, f.statsIn, f.options...)
	if err != nil {
		return nil, fmt.Errorf("incorrect OTLP metrics configuration: %w", err)
	}

	var reporter *inframetadata.Reporter
	if cfg.HostMetadata.Enabled {
		reporter, err = f.Reporter(params, forwarder, cfg.HostMetadata.ReporterPeriod)
		if err != nil {
			return nil, err
		}
	}
	newExp, err := NewExporter(f.s, cfg, f.enricher, hostGetter, f.createConsumer, tr, params, reporter)
	if err != nil {
		return nil, err
	}

	exporter, err := exporterhelper.NewMetrics(ctx, params, cfg, newExp.ConsumeMetrics,
		exporterhelper.WithQueue(cfg.QueueConfig),
		exporterhelper.WithTimeout(cfg.TimeoutConfig),
		// the metrics remapping code mutates data
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
		exporterhelper.WithShutdown(func(ctx context.Context) error {
			if cfg.ShutdownFunc != nil {
				err = cfg.ShutdownFunc(ctx)
				if err != nil {
					return err
				}
			}
			if f.wg != nil {
				f.wg.Wait() // wait for consumeStatsPayload to exit
			}
			if f.statsIn != nil {
				close(f.statsIn)
			}
			if forwarder != nil {
				forwarder.Stop()
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
