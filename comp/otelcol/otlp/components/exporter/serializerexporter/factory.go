// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/zap"

	pkgdatadog "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	otlpmetrics "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
)

const (
	// TypeStr defines the serializer exporter type string.
	TypeStr   = "serializer"
	stability = component.StabilityLevelStable
)

type factory struct {
	s            serializer.MetricSerializer
	hostProvider SourceProviderFunc

	statsIn chan []byte

	createConsumer createConsumerFunc
	options        []otlpmetrics.TranslatorOption

	onceReporter sync.Once
	reporter     *inframetadata.Reporter
	gatewayUsage otel.GatewayUsage

	ipath ingestionPath
	store TelemetryStore
}

// TelemetryStore stores the internal COAT (cross-org agent telemetry) metrics in DDOT
type TelemetryStore struct {
	// OTLPIngestMetrics tracks hosts running OTLP ingest on metrics
	OTLPIngestMetrics telemetry.Gauge
	// DDOTMetrics tracks hosts running DDOT and ingest metrics
	DDOTMetrics telemetry.Gauge
	// DDOTTraces tracks hosts running DDOT and ingest traces
	DDOTTraces telemetry.Gauge
}

type createConsumerFunc func(extraTags []string, apmReceiverAddr string, buildInfo component.BuildInfo) SerializerConsumer

// NewFactoryForAgent creates a new serializer exporter factory for Agent OTLP ingestion.
// Serializer exporter should never receive APM stats in Agent OTLP ingestion.
func NewFactoryForAgent(s serializer.MetricSerializer, hostGetter SourceProviderFunc, store TelemetryStore) exp.Factory {
	cfgType := component.MustNewType(TypeStr)
	return newFactoryForAgentWithType(s, hostGetter, nil, cfgType, otel.NewDisabledGatewayUsage(), store, nil, agentOTLPIngest)
}

// NewFactoryForOTelAgent creates a new serializer exporter factory for the embedded collector.
func NewFactoryForOTelAgent(
	s serializer.MetricSerializer,
	hostGetter SourceProviderFunc,
	statsIn chan []byte,
	gatewayusage otel.GatewayUsage,
	store TelemetryStore,
	reporter *inframetadata.Reporter,
) exp.Factory {
	cfgType := component.MustNewType("datadog") // this is called in datadog exporter (NOT serializer exporter) in embedded collector
	return newFactoryForAgentWithType(s, hostGetter, statsIn, cfgType, gatewayusage, store, reporter, ddot)
}

func newFactoryForAgentWithType(
	s serializer.MetricSerializer,
	hostGetter SourceProviderFunc,
	statsIn chan []byte,
	typ component.Type,
	gatewayUsage otel.GatewayUsage,
	store TelemetryStore,
	reporter *inframetadata.Reporter,
	ipath ingestionPath,
) exp.Factory {
	var options []otlpmetrics.TranslatorOption
	if !pkgdatadog.MetricRemappingDisabledFeatureGate.IsEnabled() {
		options = append(options, otlpmetrics.WithOTelPrefix())
	}

	f := &factory{
		s:            s,
		hostProvider: hostGetter,
		statsIn:      statsIn,
		createConsumer: func(extraTags []string, apmReceiverAddr string, _ component.BuildInfo) SerializerConsumer {
			return &serializerConsumer{
				extraTags:       extraTags,
				apmReceiverAddr: apmReceiverAddr,
				ipath:           ipath,
				hosts:           make(map[string]struct{}),
				ecsFargateTags:  make(map[string]struct{}),
			}
		},
		options:      options,
		gatewayUsage: gatewayUsage,
		ipath:        ipath,
		store:        store,
	}

	if reporter != nil {
		// reporter is initialized in datadogexporter.NewFactory in DDOT, no need to initialize it again
		f.onceReporter.Do(func() {
			f.reporter = reporter
		})
	}

	return exp.NewFactory(
		typ,
		newDefaultConfigForAgent,
		exp.WithMetrics(f.createMetricExporter, stability),
	)
}

// NewFactoryForOSSExporter creates a new serializer exporter factory for the OSS Datadog exporter.
func NewFactoryForOSSExporter(typ component.Type, statsIn chan []byte) exp.Factory {
	var options []otlpmetrics.TranslatorOption
	if !pkgdatadog.MetricRemappingDisabledFeatureGate.IsEnabled() {
		options = append(options, otlpmetrics.WithRemapping())
	}

	f := &factory{
		// hostProvider is a no-op function that returns an empty host.
		// In OSS collector, the host is overridden via the HostProvider field in the config.
		hostProvider: func(_ context.Context) (string, error) { return "", nil },
		createConsumer: func(extraTags []string, apmReceiverAddr string, buildInfo component.BuildInfo) SerializerConsumer {
			s := &serializerConsumer{extraTags: extraTags, apmReceiverAddr: apmReceiverAddr, ipath: ossCollector}
			return &collectorConsumer{
				serializerConsumer: s,
				seenHosts:          make(map[string]struct{}),
				seenTags:           make(map[string]struct{}),
				buildInfo:          buildInfo,
				getPushTime:        func() uint64 { return uint64(time.Now().Unix()) },
			}
		},
		options: options,
		statsIn: statsIn,
		ipath:   ossCollector,
	}
	return exp.NewFactory(
		typ,
		newDefaultConfig,
		exp.WithMetrics(f.createMetricExporter, stability),
	)
}

// NewFactory implements the required func to be used in OCB. This interface does not work with APM stats. Do not change the func signature or OCB will fail.
func NewFactory() exp.Factory {
	return NewFactoryForOSSExporter(component.MustNewType(TypeStr), nil)
}

// Reporter builds and returns an *inframetadata.Reporter.
func (f *factory) Reporter(params exp.Settings, s serializer.MetricSerializer, reporterPeriod time.Duration) (*inframetadata.Reporter, error) {
	var reporterErr error
	f.onceReporter.Do(func() {
		f.reporter, reporterErr = inframetadata.NewReporter(params.Logger, NewPusher(s), reporterPeriod)
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

// checkAndCastConfig checks the configuration type and its warnings, and casts it to
// the Datadog Config struct.
func checkAndCastConfig(c component.Config, logger *zap.Logger) (*ExporterConfig, error) {
	cfg, ok := c.(*ExporterConfig)
	if !ok {
		return nil, errors.New("programming error: config structure is not of type *ExporterConfig")
	}
	cfg.LogWarnings(logger)
	return cfg, nil
}

// createMetricsExporter creates a new metrics exporter.
func (f *factory) createMetricExporter(ctx context.Context, params exp.Settings, c component.Config) (exp.Metrics, error) {
	cfg, err := checkAndCastConfig(c, params.Logger)
	if err != nil {
		return nil, err
	}
	var forwarder *defaultforwarder.DefaultForwarder
	if f.s == nil {
		f.s, forwarder, err = InitSerializer(params.Logger, cfg, f.hostProvider)
		if err != nil {
			return nil, err
		}
		params.Logger.Info("starting forwarder")
		err := forwarder.Start()
		if err != nil {
			params.Logger.Error("failed to start forwarder", zap.Error(err))
		}
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
		reporter, err = f.Reporter(params, f.s, cfg.HostMetadata.ReporterPeriod)
		if err != nil {
			return nil, err
		}
	}

	var usageMetric telemetry.Gauge
	if f.ipath == agentOTLPIngest {
		usageMetric = f.store.OTLPIngestMetrics
	} else if f.ipath == ddot {
		usageMetric = f.store.DDOTMetrics
	}

	newExp, err := NewExporter(f.s, cfg, hostGetter, f.createConsumer, tr, params, reporter, f.gatewayUsage, usageMetric)
	if err != nil {
		return nil, err
	}

	exporter, err := exporterhelper.NewMetrics(ctx, params, cfg, newExp.ConsumeMetrics,
		exporterhelper.WithQueue(cfg.QueueBatchConfig),
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
