// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"go.uber.org/fx"
)

type workloadmeta struct {
	log    log.Component
	config config.Component
}

type dependencies struct {
	fx.In

	Log    log.Component
	Config config.Component
}

func newWorkloadMeta(lc fx.Lifecycle, deps dependencies) Component {
	var catalog CollectorCatalog
	if flavor.GetFlavor() == flavor.ClusterAgent {
		catalog = ClusterAgentCatalog
	} else {
		catalog = NodeAgentCatalog
	}

	store := CreateGlobalStore(catalog)
	store.Start(ctx)
}
