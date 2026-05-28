// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the Synthetics team's event platform pipeline
// description to the event platform forwarder via the "ep_pipeline_descs" fx group.
package fx

// team: synthetics-executing

import (
	"go.uber.org/fx"

	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	syntheticseventplatform "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/eventplatform"
)

// Module returns the fx module that registers the Synthetics pipeline description.
func Module() fx.Option {
	return fx.Module(
		"comp/syntheticstestscheduler/eventplatform",
		fx.Provide(fx.Annotate(
			func(cfg compconfig.Component) []eventplatform.PipelineDesc {
				return syntheticseventplatform.Descs(cfg)
			},
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
