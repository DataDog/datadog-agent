// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the Network Path team's event platform pipeline
// descriptions to the event platform forwarder via the "ep_pipeline_descs" fx group.
package fx

// team: network-path

import (
	"go.uber.org/fx"

	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	networkpatheventplatfrom "github.com/DataDog/datadog-agent/comp/networkpath/eventplatfrom"
)

// Module returns the fx module that registers the Network Path pipeline description.
func Module() fx.Option {
	return fx.Module(
		"comp/networkpath/eventplatfrom",
		fx.Provide(fx.Annotate(
			func(cfg compconfig.Component) []eventplatform.PipelineDesc {
				return networkpatheventplatfrom.Descs(cfg)
			},
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
