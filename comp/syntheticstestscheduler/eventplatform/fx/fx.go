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

	syntheticseventplatform "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/eventplatform"
)

// Module returns the fx module that registers the Synthetics pipeline description.
//
// The slice returned by syntheticseventplatform.Descs() is flattened into the fx
// group, so each PipelineDesc becomes one element of the []eventplatform.PipelineDesc
// consumed by the event platform forwarder.
func Module() fx.Option {
	return fx.Module(
		"comp/syntheticstestscheduler/eventplatform",
		fx.Provide(fx.Annotate(
			syntheticseventplatform.Descs,
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
