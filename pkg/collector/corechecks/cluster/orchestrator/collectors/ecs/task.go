// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package ecs defines a collector to collect ECS task
package ecs

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/ecs"
	transformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/ecs"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TaskCollector is a collector for ECS tasks.
type TaskCollector struct {
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewTaskCollector creates a new collector for the ECS Task resource.
func NewTaskCollector(tagger tagger.Component) *TaskCollector {
	return &TaskCollector{
		metadata: &collectors.CollectorMetadata{
			IsStable:           false,
			IsMetadataProducer: true,
			IsManifestProducer: false,
			Name:               "ecstasks",
			NodeType:           orchestrator.ECSTask,
		},
		processor: processors.NewProcessor(ecs.NewTaskHandlers(tagger)),
	}
}

// Metadata is used to access information about the collector.
func (t *TaskCollector) Metadata() *collectors.CollectorMetadata {
	return t.metadata
}

// Init is used to initialize the collector.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (t *TaskCollector) Init(rcfg *collectors.CollectorRunConfig) {}

// Run triggers the collection process.
func (t *TaskCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list := rcfg.WorkloadmetaStore.ListECSTasks()
	tasks := make([]transformers.TaskWithContainers, 0, len(list))
	for _, task := range list {
		newTask := task
		tasks = append(tasks, t.fetchContainers(rcfg, newTask))
	}

	ctx := &processors.ECSProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              rcfg.Config,
			MsgGroupID:       rcfg.MsgGroupRef.Inc(),
			NodeType:         t.metadata.NodeType,
			ManifestProducer: t.metadata.IsManifestProducer,
			ClusterID:        rcfg.ClusterID,
		},
		AWSAccountID: rcfg.AWSAccountID,
		ClusterName:  rcfg.ClusterName,
		Region:       rcfg.Region,
		SystemInfo:   rcfg.SystemInfo,
		Hostname:     rcfg.HostName,
	}

	processResult, processed := t.processor.Process(ctx, tasks)

	if processed == -1 {
		return nil, fmt.Errorf("unable to process resources: a panic occurred")
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    len(list),
		ResourcesProcessed: processed,
	}

	return result, nil
}

// fetchContainers fetches the containers from workloadmeta store for a given task.
func (t *TaskCollector) fetchContainers(rcfg *collectors.CollectorRunConfig, task *workloadmeta.ECSTask) transformers.TaskWithContainers {
	ecsTask := transformers.TaskWithContainers{
		Task:       task,
		Containers: make([]*workloadmeta.Container, 0, len(task.Containers)),
	}

	for _, container := range task.Containers {
		c, err := rcfg.WorkloadmetaStore.GetContainer(container.ID)
		if err != nil {
			log.Errorc(err.Error(), orchestrator.ExtraLogContext...)
			continue
		}
		ecsTask.Containers = append(ecsTask.Containers, c)
	}

	return ecsTask
}
