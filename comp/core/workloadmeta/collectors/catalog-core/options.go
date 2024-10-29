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

	cfcontainer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/container"
	cfvm "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/vm"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/containerd"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/docker"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecs"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecsfargate"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubeapiserver"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubelet"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubemetadata"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/podman"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/process"
	remoteprocesscollector "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/processcollector"
)

func getCollectorOptions() []fx.Option {
	return []fx.Option{
		cfcontainer.GetFxOptions(),
		cfvm.GetFxOptions(),
		containerd.GetFxOptions(),
		docker.GetFxOptions(),
		ecs.GetFxOptions(),
		ecsfargate.GetFxOptions(),
		kubeapiserver.GetFxOptions(),
		kubelet.GetFxOptions(),
		kubemetadata.GetFxOptions(),
		podman.GetFxOptions(),
		remoteprocesscollector.GetFxOptions(),
		process.GetFxOptions(),
	}
}
