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

	softinveventplatform "github.com/DataDog/datadog-agent/comp/softwareinventory/eventplatform"
)

// Module returns the fx module that registers the Software Inventory pipeline description.
//
// The slice returned by softinveventplatform.Descs() is flattened into the fx
// group. Descs() returns nil when software_inventory.enabled is false, so no
// pipeline is registered in that case.
func Module() fx.Option {
	return fx.Module(
		"comp/softwareinventory/eventplatform",
		fx.Provide(fx.Annotate(
			softinveventplatform.Descs,
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
