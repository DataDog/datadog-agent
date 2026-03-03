// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"sync"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/collector/processor/processorhelper/xprocessorhelper"
	"go.opentelemetry.io/collector/processor/xprocessor"

	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

type factory struct {
	data *data // Must be accessed only through getOrCreateData
	mu   sync.Mutex
}

type data struct {
	infraTags infraTagsProcessor
}

func (f *factory) getOrCreateData() (*data, error) {
	// Ensure that the tagger is initialized only once.
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data != nil {
		return f.data, nil
	}
	f.data = &data{}
	var client taggerTypes.TaggerClient
	app := fx.New(
		fx.Provide(func() config.Component {
			return pkgconfigsetup.Datadog()
		}),
		fx.Provide(func(_ config.Component) log.Params {
			return log.ForDaemon("otelcol", "log_file", pkgconfigsetup.DefaultOTelAgentLogFile)
		}),
		logfx.Module(),
		telemetryModule(),
		fxutil.FxAgentBase(),
		remoteTaggerfx.Module(tagger.NewRemoteParams()),
		fx.Provide(func(t tagger.Component) taggerTypes.TaggerClient {
			return t
		}),
		fx.Populate(&client),
	)
	if err := app.Err(); err != nil {
		return nil, err
	}
	f.data.infraTags = newInfraTagsProcessor(client, option.None[SourceProviderFunc]())
	return f.data, nil
}

// NewFactory returns a new factory for the InfraAttributes processor.
func NewFactory() processor.Factory {
	return newFactoryForAgent(nil)
}

// SourceProviderFunc is a function that returns the source of the host.
type SourceProviderFunc func(context.Context) (string, error)

// NewFactoryForAgent returns a new factory for the InfraAttributes processor.
func NewFactoryForAgent(tagger taggerTypes.TaggerClient, hostGetter SourceProviderFunc) xprocessor.Factory {
	return newFactoryForAgent(&data{
		infraTags: newInfraTagsProcessor(tagger, option.New(hostGetter)),
	})
}

func newFactoryForAgent(data *data) xprocessor.Factory {
	f := &factory{
		data: data,
	}

	return xprocessor.NewFactory(
		Type,
		f.createDefaultConfig,
		xprocessor.WithMetrics(f.createMetricsProcessor, MetricsStability),
		xprocessor.WithLogs(f.createLogsProcessor, LogsStability),
		xprocessor.WithTraces(f.createTracesProcessor, TracesStability),
		xprocessor.WithProfiles(f.createProfilesProcessor, ProfilesStability),
	)
}

func (f *factory) createDefaultConfig() component.Config {
	return &Config{
		Cardinality: taggerTypes.LowCardinality,
	}
}

func (f *factory) createMetricsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (processor.Metrics, error) {
	data, err := f.getOrCreateData()
	if err != nil {
		return nil, err
	}

	iap, err := newInfraAttributesMetricProcessor(set, data.infraTags, cfg.(*Config))
	if err != nil {
		return nil, err
	}
	return processorhelper.NewMetrics(
		ctx,
		set,
		cfg,
		nextConsumer,
		iap.processMetrics,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createLogsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (processor.Logs, error) {
	data, err := f.getOrCreateData()
	if err != nil {
		return nil, err
	}
	iap, err := newInfraAttributesLogsProcessor(set, data.infraTags, cfg.(*Config))
	if err != nil {
		return nil, err
	}
	return processorhelper.NewLogs(
		ctx,
		set,
		cfg,
		nextConsumer,
		iap.processLogs,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	data, err := f.getOrCreateData()
	if err != nil {
		return nil, err
	}
	iap, err := newInfraAttributesSpanProcessor(set, data.infraTags, cfg.(*Config))
	if err != nil {
		return nil, err
	}
	return processorhelper.NewTraces(
		ctx,
		set,
		cfg,
		nextConsumer,
		iap.processTraces,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createProfilesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer xconsumer.Profiles,
) (xprocessor.Profiles, error) {
	data, err := f.getOrCreateData()
	if err != nil {
		return nil, err
	}
	iap, err := newInfraAttributesProfileProcessor(set, data.infraTags, cfg.(*Config))
	if err != nil {
		return nil, err
	}
	return xprocessorhelper.NewProfiles(
		ctx,
		set,
		cfg,
		nextConsumer,
		iap.processProfiles,
		xprocessorhelper.WithCapabilities(processorCapabilities))
}
