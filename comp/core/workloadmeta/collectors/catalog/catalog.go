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

	"github.com/DataDog/datadog-agent/comp/core/config"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
	remoteworkloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// GetCatalog returns the set of collectors in the catalog
func GetCatalog(cfg config.Component) []wmcatalog.Collector {
	return getCollectorList(cfg)
}

// TODO: (components) Move remote-only to its own catalog, similar to how catalog-less works
// Depend on this catalog-remote using fx, instead of build tags

func remoteWorkloadmetaParams() fx.Option {
	var filter *workloadmeta.Filter // Nil filter accepts everything

	// Security Agent is only interested in containers
	// TODO: (components) create a Catalog component, the implementation used by
	// security-agent can use this filter, instead of needing to chekc agent.flavor
	if flavor.GetFlavor() == flavor.SecurityAgent {
		filter = workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindContainer).Build()
	}

	return fx.Provide(func() remoteworkloadmeta.Params {
		return remoteworkloadmeta.Params{
			Filter: filter,
		}
	})
}
