// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-otel"

	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func initSerializer(logger *zap.Logger, cfg *ExporterConfig, sourceProvider source.Provider) (*serializer.Serializer, *defaultforwarder.DefaultForwarder, error) {
	var f defaultforwarder.Component
	var s *serializer.Serializer
	app := fx.New(
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log}
		}),
		fx.Supply(logger),
		fxutil.FxAgentBase(),
		fx.Provide(func() config.Component {
			pkgconfig := pkgconfigmodel.NewConfig("DD", "DD", strings.NewReplacer(".", "_"))

			// Set the API Key
			pkgconfig.Set("api_key", string(cfg.API.Key), pkgconfigmodel.SourceFile)
			pkgconfig.Set("site", cfg.API.Site, pkgconfigmodel.SourceFile)

			return pkgconfigsetup.Datadog()
		}),
		fx.Provide(func(log *zap.Logger) (logdef.Component, error) {
			return &zaplogger{logger: log}, nil
		}),

		//fx.Provide(func(c coreconfig.Component, l corelog.Component) (defaultforwarder.Params, error) {
		//	return defaultforwarder.NewParams()	, nil
		//}),
		// casts the defaultforwarder.Component to a defaultforwarder.Forwarder
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		// this is the hostname argument for serializer.NewSerializer
		// this should probably be wrapped by a type
		fx.Provide(func() string {
			s, err := sourceProvider.Source(context.TODO())
			if err != nil {
				return ""
			}
			return s.Identifier
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Provide(serializer.NewSerializer),
		// fx.Provide(strategy.NewZlibStrategy),
		// this doesn't let us switch impls.........
		metricscompressionfx.Module(),
		// casts the metricscompression.Component to a compression.Compressor
		fx.Provide(func(c metricscompression.Component) compression.Compressor {
			return c
		}),
		//fx.Provide(func(s *strategy.ZlibStrategy) compression.Component {
		//	return s
		//}),
		defaultforwarder.Module(defaultforwarder.NewParams()),
		fx.Populate(&f),
		fx.Populate(&s),
	)
	if err := app.Err(); err != nil {
		return nil, nil, err
	}
	fw, ok := f.(*defaultforwarder.DefaultForwarder)
	if !ok {
		return nil, nil, fmt.Errorf("failed to cast forwarder to defaultforwarder.DefaultForwarder")
	}
	return s, fw, nil
}

type orchestratorinterfaceimpl struct {
	f defaultforwarder.Forwarder
}

func newOrchestratorinterfaceimpl(f defaultforwarder.Forwarder) orchestratorinterface.Component {
	return &orchestratorinterfaceimpl{
		f: f,
	}
}

func (o *orchestratorinterfaceimpl) Get() (defaultforwarder.Forwarder, bool) {
	return o.f, true
}
