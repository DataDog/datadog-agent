// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package run

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
)

func getTaggerModule() fx.Option {
	return fx.Options(
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		fx.Provide(func(config config.Component, log log.Component, wmeta workloadmeta.Component, telemetry telemetry.Component) tagger.Component {
			provides := taggerimpl.NewMock(taggerimpl.MockRequires{
				Config:       config,
				WorkloadMeta: wmeta,
				Log:          log,
				Telemetry:    telemetry,
			})
			return provides.Comp
		}),
	)
}
