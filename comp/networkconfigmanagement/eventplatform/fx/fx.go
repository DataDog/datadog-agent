// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the NDM integrations team's Network Config Management
// event platform pipeline description to the event platform forwarder via the
// "ep_pipeline_descs" fx group.
package fx

// team: ndm-integrations

import (
	"go.uber.org/fx"

	ncmeventplatform "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/eventplatform"
)

// Module returns the fx module that registers the NCM pipeline description.
//
// The slice returned by ncmeventplatform.Descs() is flattened into the fx group,
// so each PipelineDesc becomes one element of the []eventplatform.PipelineDesc
// consumed by the event platform forwarder.
func Module() fx.Option {
	return fx.Module(
		"comp/networkconfigmanagement/eventplatform",
		fx.Provide(fx.Annotate(
			ncmeventplatform.Descs,
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
