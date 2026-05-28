// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the DBM team's event platform pipeline descriptions to
// the event platform forwarder via the "ep_pipeline_descs" fx group.
package fx

import (
	"go.uber.org/fx"

	dbmeventplatform "github.com/DataDog/datadog-agent/pkg/databasemonitoring/eventplatform"
)

// team: database-monitoring

// Module returns the fx module that registers the DBM pipeline descriptions.
//
// The slice returned by dbmeventplatform.Descs() is flattened into the fx group,
// so each PipelineDesc becomes one element of the []eventplatform.PipelineDesc
// consumed by the event platform forwarder.
func Module() fx.Option {
	return fx.Module(
		"pkg/databasemonitoring/eventplatform",
		fx.Provide(fx.Annotate(
			dbmeventplatform.Descs,
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
