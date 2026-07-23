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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
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
			Rootowner:   resolveRootOwner(pod.Owners),
		},
	}
}

// resolveRootOwner determines the root owner of a pod by walking the owner chain.
// For example, a pod owned by a ReplicaSet resolves to the parent Deployment.
func resolveRootOwner(owners []workloadmeta.KubernetesPodOwner) *core.FilterRootOwner {
	if len(owners) == 0 {
		return nil
	}

	owner := owners[0]
	for _, o := range owners {
		if o.Controller != nil && *o.Controller {
			owner = o
			break
		}
	}

	switch owner.Kind {
	case kubernetes.ReplicaSetKind:
		if deployment := kubernetes.ParseDeploymentForReplicaSet(owner.Name); deployment != "" {
			return &core.FilterRootOwner{Kind: kubernetes.DeploymentKind, Name: deployment}
		}
		return &core.FilterRootOwner{Kind: owner.Kind, Name: owner.Name}
	case kubernetes.JobKind:
		if cronjob, _ := kubernetes.ParseCronJobForJob(owner.Name); cronjob != "" {
			return &core.FilterRootOwner{Kind: kubernetes.CronJobKind, Name: cronjob}
		}
		return &core.FilterRootOwner{Kind: owner.Kind, Name: owner.Name}
	case kubernetes.DeploymentKind, kubernetes.DaemonSetKind, kubernetes.StatefulSetKind:
		return &core.FilterRootOwner{Kind: owner.Kind, Name: owner.Name}
	default:
		return &core.FilterRootOwner{Kind: owner.Kind, Name: owner.Name}
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
