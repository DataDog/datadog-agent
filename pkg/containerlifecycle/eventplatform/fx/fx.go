// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the container integrations team's event platform
// pipeline descriptions (lifecycle, images, SBOM) to the event platform
// forwarder via the "ep_pipeline_descs" fx group.
package fx

// team: container-integrations

import (
	"go.uber.org/fx"

	containereventplatform "github.com/DataDog/datadog-agent/pkg/containerlifecycle/eventplatform"
)

// Module returns the fx module that registers the container pipeline descriptions.
//
// The slice returned by containereventplatform.Descs() is flattened into the fx
// group, so each PipelineDesc becomes one element of the []eventplatform.PipelineDesc
// consumed by the event platform forwarder.
func Module() fx.Option {
	return fx.Module(
		"pkg/containerlifecycle/eventplatform",
		fx.Provide(fx.Annotate(
			containereventplatform.Descs,
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
