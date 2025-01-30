// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"go.uber.org/fx"

	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

type factory struct {
	tagger taggerClient
	mu     sync.Mutex
}

func (f *factory) initializeTaggerClient() error {
	// Ensure that the tagger is initialized only once.
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tagger != nil {
		return nil
	}
	var client taggerClient
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
		remoteTaggerfx.Module(tagger.RemoteParams{
			RemoteTarget: func(c config.Component) (string, error) {
				return fmt.Sprintf(":%v", c.GetInt("cmd_port")), nil
			},
			RemoteTokenFetcher: func(c config.Component) func() (string, error) {
				return func() (string, error) {
					return security.FetchAuthToken(c)
				}
			},
			RemoteFilter: taggerTypes.NewMatchAllFilter(),
		}),
		fx.Provide(func(t tagger.Component) taggerClient {
			return t
		}),
		fx.Populate(&client),
	)
	if err := app.Err(); err != nil {
		return err
	}
	f.tagger = client
	return nil
}

// NewFactory returns a new factory for the InfraAttributes processor.
func NewFactory() processor.Factory {
	return NewFactoryForAgent(nil)
}

// NewFactoryForAgent returns a new factory for the InfraAttributes processor.
func NewFactoryForAgent(tagger taggerClient) processor.Factory {
	f := &factory{
		tagger: tagger,
	}

	return processor.NewFactory(
		Type,
		f.createDefaultConfig,
		processor.WithMetrics(f.createMetricsProcessor, MetricsStability),
		processor.WithLogs(f.createLogsProcessor, LogsStability),
		processor.WithTraces(f.createTracesProcessor, TracesStability),
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
	if f.tagger == nil {
		err := f.initializeTaggerClient()
		if err != nil {
			return nil, err
		}
	}
	iap, err := newInfraAttributesMetricProcessor(set, cfg.(*Config), f.tagger)
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
	if f.tagger == nil {
		err := f.initializeTaggerClient()
		if err != nil {
			return nil, err
		}
	}
	iap, err := newInfraAttributesLogsProcessor(set, cfg.(*Config), f.tagger)
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
	if f.tagger == nil {
		err := f.initializeTaggerClient()
		if err != nil {
			return nil, err
		}
	}
	iap, err := newInfraAttributesSpanProcessor(set, cfg.(*Config), f.tagger)
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
