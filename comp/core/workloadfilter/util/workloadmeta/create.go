// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadmeta contains utility functions for creating filterable objects.
package workloadmeta

import (
	"strings"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// CreateContainer creates a Filterable Container object from a workloadmeta.Container and an owner.
func CreateContainer(container *workloadmeta.Container, owner workloadfilter.Filterable) *workloadfilter.Container {
	if container == nil {
		return nil
	}
	return workloadfilter.CreateContainer(container.ID, container.Name, container.Image.RawName, owner)
}

// CreateContainerFromOrch creates a Filterable Container object from a workloadmeta.OrchestratorContainer and an owner.
func CreateContainerFromOrch(container *workloadmeta.OrchestratorContainer, owner workloadfilter.Filterable) *workloadfilter.Container {
	if container == nil {
		return nil
	}
	return workloadfilter.CreateContainer(container.ID, container.Name, container.Image.RawName, owner)
}

// CreatePod creates a Filterable Pod object from a workloadmeta.KubernetesPod.
func CreatePod(pod *workloadmeta.KubernetesPod) *workloadfilter.Pod {
	if pod == nil {
		return nil
	}

	return &workloadfilter.Pod{
		FilterPod: &core.FilterPod{
			Id:          pod.ID,
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: pod.Annotations,
		},
	}
}

// CreateProcess creates a Filterable Process object from a workloadmeta.Process.
func CreateProcess(process *workloadmeta.Process) *workloadfilter.Process {
	if process == nil {
		return nil
	}

	p := &core.FilterProcess{
		Name:    process.Name,
		Cmdline: strings.Join(process.Cmdline, " "),
		Args:    process.Cmdline,
	}

	return &workloadfilter.Process{
		FilterProcess: p,
	}
}
