// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog is a wrapper that loads the available workloadmeta
// collectors. It exists as a shorthand for importing all packages manually in
// all of the agents.
package catalog

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
	cfcontainer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/container"
	cfvm "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/cloudfoundry/vm"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/containerd"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/docker"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecs"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecsfargate"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/host"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubeapiserver"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubelet"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubemetadata"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/podman"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/process"
	remoteprocesscollector "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/processcollector"
)

func firstArg(c wmcatalog.Collector, _ error) wmcatalog.Collector {
	return c
}

func getCollectorList(cfg config.Component) []wmcatalog.Collector {
	return []wmcatalog.Collector{
		firstArg(cfcontainer.NewCollector(cfg)),
		firstArg(cfvm.NewCollector(cfg)),
		firstArg(containerd.NewCollector(cfg)),
		firstArg(docker.NewCollector(cfg)),
		firstArg(ecs.NewCollector(cfg)),
		firstArg(ecsfargate.NewCollector(cfg)),
		firstArg(kubeapiserver.NewCollector(cfg)),
		firstArg(kubelet.NewCollector(cfg)),
		firstArg(kubemetadata.NewCollector(cfg)),
		firstArg(podman.NewCollector(cfg)),
		firstArg(remoteprocesscollector.NewCollector(cfg)),
		firstArg(host.NewCollector(cfg)),
		firstArg(process.NewCollector(cfg)),
	}
}
