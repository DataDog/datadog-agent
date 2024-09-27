// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build remotewmonly

// Package collectors is a wrapper that loads the available workloadmeta
// collectors. It exists as a shorthand for importing all packages manually in
// all of the agents.
package catalog

import (
	"go.uber.org/fx"

	remoteworkloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/workloadmeta"
)

// TODO: (components) Move remote-only to its own catalog, similar to how catalog-less works
// Depend on this catalog-remote using fx, instead of build tags

func getCollectorOptions() []fx.Option {
	return []fx.Option{
		remoteworkloadmeta.GetFxOptions(),
		remoteWorkloadmetaParams(),
	}
}
