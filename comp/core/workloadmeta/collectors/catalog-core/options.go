// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog is a wrapper that loads the available workloadmeta
// collectors. This is the catalog used by the core agent.
package catalog

import (
	"go.uber.org/fx"

	cfcontainer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/container"
	cfvm "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/vm"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/containerd"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/crio"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/docker"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecs"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecsfargate"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubelet"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubemetadata"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/nvml"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/podman"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/process"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/processlanguage"
	remoteprocesscollector "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/processcollector"
)

func getCollectorOptions() []fx.Option {
	return []fx.Option{
		cfcontainer.GetFxOptions(),
		cfvm.GetFxOptions(),
		containerd.GetFxOptions(),
		crio.GetFxOptions(),
		docker.GetFxOptions(),
		ecs.GetFxOptions(),
		ecsfargate.GetFxOptions(),
		kubelet.GetFxOptions(),
		kubemetadata.GetFxOptions(),
		podman.GetFxOptions(),
		remoteprocesscollector.GetFxOptions(),
		processlanguage.GetFxOptions(),
		nvml.GetFxOptions(),
		process.GetFxOptions(),
	}
}
