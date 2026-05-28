// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the Data Observability team's event platform pipeline
// description to the event platform forwarder via the "ep_pipeline_descs" fx group.
package fx

// team: data-observability

import (
	"go.uber.org/fx"

	doeventplatform "github.com/DataDog/datadog-agent/comp/dataobs/queryactions/eventplatform"
)

// Module returns the fx module that registers the Data Observability pipeline description.
//
// The slice returned by doeventplatform.Descs() is flattened into the fx group,
// so each PipelineDesc becomes one element of the []eventplatform.PipelineDesc
// consumed by the event platform forwarder.
func Module() fx.Option {
	return fx.Module(
		"comp/dataobs/queryactions/eventplatform",
		fx.Provide(fx.Annotate(
			doeventplatform.Descs,
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
