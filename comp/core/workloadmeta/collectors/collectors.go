// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectors is a wrapper that loads the available workloadmeta
// collectors. It exists as a shorthand for importing all packages manually in
// all of the agents.
package collectors

import (
	"go.uber.org/fx"

	cf_container "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/container"
	cf_vm "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/vm"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/containerd"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/docker"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecs"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecsfargate"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/host"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubeapiserver"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubelet"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubemetadata"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/podman"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/processcollector"
	remoteworkloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// GetCatalog returns the set of FX options to populate the catalog
func GetCatalog() fx.Option {
	options := []fx.Option{
		cf_container.GetFxOptions(),
		cf_vm.GetFxOptions(),
		containerd.GetFxOptions(),
		docker.GetFxOptions(),
		ecs.GetFxOptions(),
		ecsfargate.GetFxOptions(),
		kubeapiserver.GetFxOptions(),
		kubelet.GetFxOptions(),
		kubemetadata.GetFxOptions(),
		podman.GetFxOptions(),
		remoteworkloadmeta.GetFxOptions(),
		remoteWorkloadmetaParams(),
		processcollector.GetFxOptions(),
		host.GetFxOptions(),
	}

	// remove nil options
	opts := make([]fx.Option, 0, len(options))
	for _, item := range options {
		if item != nil {
			opts = append(opts, item)
		}
	}
	return fx.Options(opts...)
}

func remoteWorkloadmetaParams() fx.Option {
	var filter *workloadmeta.Filter // Nil filter accepts everything

	// Security Agent is only interested in containers
	if flavor.GetFlavor() == flavor.SecurityAgent {
		filter = workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindContainer).Build()
	}

	return fx.Provide(func() remoteworkloadmeta.Params {
		return remoteworkloadmeta.Params{
			Filter: filter,
		}
	})
}
