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
)

// GetCatalog returns the set of FX options to populate the catalog
func GetCatalog() fx.Option {
	options := getCollectorOptions()

	// remove nil options
	opts := make([]fx.Option, 0, len(options))
	for _, item := range options {
		if item != nil {
			opts = append(opts, item)
		}
	}
	return fx.Options(opts...)
}

// TODO: (components) Move remote-only to its own catalog, similar to how catalog-less works
// Depend on this catalog-remote using fx, instead of build tags

func remoteWorkloadmetaParams() fx.Option {
	return fx.Provide(func() remoteworkloadmeta.Params {
		return remoteworkloadmeta.Params{} // Nil filter accepts everything
	})
}
