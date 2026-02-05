// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog is a wrapper that loads the available workloadmeta
// collectors. It exists as a shorthand for importing all packages manually in
// all of the agents.
package catalog

import (
	"go.uber.org/fx"

	remoteworkloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func getCollectorOptions() []fx.Option {
	return []fx.Option{
		remoteworkloadmeta.GetFxOptions(),
		remoteWorkloadmetaParams(),
	}
}

func remoteWorkloadmetaParams() fx.Option {
	return fx.Provide(func() remoteworkloadmeta.Params {
		filter := workloadmeta.NewFilterBuilder().
			AddKind(workloadmeta.KindContainer).
			AddKind(workloadmeta.KindKubernetesPod).
			AddKind(workloadmeta.KindECSTask).
			AddKind(workloadmeta.KindProcess).
			AddKind(workloadmeta.KindContainerImageMetadata).
			Build()

		return remoteworkloadmeta.Params{
			Filter: filter,
		}
	})
}
