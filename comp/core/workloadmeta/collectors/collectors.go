// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectors is a wrapper that loads the available workloadmeta
// collectors. It exists as a shorthand for importing all packages manually in
// all of the agents.
package collectors

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/cf_container"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/cf_vm"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/containerd"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/docker"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecs"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecsfargate"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubeapiserver"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubelet"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubemetadata"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/podman"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/processcollector"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/workloadmeta"
	"go.uber.org/fx"
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
		workloadmeta.GetFxOptions(),
		processcollector.GetFxOptions(),
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
