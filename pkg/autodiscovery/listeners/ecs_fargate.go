// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

// +build !serverless

package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func init() {
	Register("ecs", NewECSFargateListener)
}

// ECSFargateListener listens to container creation through a subscription to
// the workloadmeta store.
type ECSFargateListener struct {
	workloadmetaListener
}

// NewECSFargateListener returns a new ECSFargateListener.
func NewECSFargateListener() (ServiceListener, error) {
	const name = "ad-ecsfargatelistener"
	l := &ECSFargateListener{}
	f := workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindECSTask},
		[]workloadmeta.Source{workloadmeta.SourceECSFargate},
	)

	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, f, l.processFargateTask)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *ECSFargateListener) processFargateTask(
	entity workloadmeta.Entity,
	creationTime integration.CreationTime,
) {
	task := entity.(*workloadmeta.ECSTask)

	for _, taskContainer := range task.Containers {
		container, err := l.Store().GetContainer(taskContainer.ID)
		if err != nil {
			log.Debugf("task %q has reference to non-existing container %q", task.Name, taskContainer.ID)
			continue
		}

		l.createContainerService(task, container, creationTime)
	}
}

func (l *ECSFargateListener) createContainerService(
	task *workloadmeta.ECSTask,
	container *workloadmeta.Container,
	creationTime integration.CreationTime,
) {
	if !container.State.Running {
		return
	}

	containerImg := container.Image
	if l.IsExcluded(
		containers.GlobalFilter,
		container.Name,
		containerImg.RawName,
		"",
	) {
		log.Debugf("container %s filtered out: name %q image %q", container.ID, container.Name, container.Image.RawName)
		return
	}

	hosts := make(map[string]string)
	for host, ip := range container.NetworkIPs {
		hosts[host] = ip
	}

	svc := &service{
		entity: container,
		adIdentifiers: ComputeContainerServiceIDs(
			containers.BuildEntityName(string(container.Runtime), container.ID),
			containerImg.RawName,
			container.Labels,
		),
		creationTime: creationTime,
		hosts:        hosts,
		metricsExcluded: l.IsExcluded(
			containers.MetricsFilter,
			container.Name,
			containerImg.RawName,
			"",
		),
		logsExcluded: l.IsExcluded(
			containers.LogsFilter,
			container.Name,
			containerImg.RawName,
			"",
		),
		ready: true,
	}

	var err error
	svc.checkNames, err = getCheckNamesFromLabels(container.Labels)
	if err != nil {
		log.Errorf("error getting check names from docker labels on container %s: %v", container.ID, err)
	}

	svcID := buildSvcID(container.GetID())
	taskSvcID := buildSvcID(task.GetID())
	l.AddService(svcID, svc, taskSvcID)
}
