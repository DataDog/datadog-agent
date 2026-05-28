// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the Windows Products team's Software Inventory event
// platform pipeline description to the event platform forwarder via the
// "ep_pipeline_descs" fx group.
package fx

// team: windows-products

import (
	"go.uber.org/fx"

	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	softinveventplatform "github.com/DataDog/datadog-agent/comp/softwareinventory/eventplatform"
)

// Module returns the fx module that registers the Software Inventory pipeline description.
// Descs returns nil when software_inventory.enabled is false, so no pipeline is registered.
func Module() fx.Option {
	return fx.Module(
		"comp/softwareinventory/eventplatform",
		fx.Provide(fx.Annotate(
			func(cfg compconfig.Component) []eventplatform.PipelineDesc {
				return softinveventplatform.Descs(cfg)
			},
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
