// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package collectors

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
)

// processServiceList process a service list into process messages.
func processServiceList(serviceList []*corev1.Service, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, int) {
	serviceMsgs := make([]*model.Service, 0, len(serviceList))

	for s := 0; s < len(serviceList); s++ {
		svc := serviceList[s]
		if orchestrator.SkipKubernetesResource(svc.UID, svc.ResourceVersion, orchestrator.K8sService) {
			continue
		}
		redact.RemoveLastAppliedConfigurationAnnotation(svc.Annotations)

		serviceModel := extractService(svc)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonSvc, err := jsoniter.Marshal(svc)
		if err != nil {
			log.Warnf("Could not marshal to JSON: %s", err)
			continue
		}
		serviceModel.Yaml = jsonSvc

		serviceMsgs = append(serviceMsgs, serviceModel)
	}

	groupSize := orchestrator.GroupSize(len(serviceMsgs), cfg.MaxPerMessage)

	chunks := chunkServices(serviceMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorService{
			ClusterName: cfg.KubeClusterName,
			ClusterId:   clusterID,
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			Services:    chunks[i],
			Tags:        cfg.ExtraTags,
		})
	}

	return messages, len(serviceMsgs)
}

// chunkServices chunks the given list of services, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkServices(services []*model.Service, chunkCount, chunkSize int) [][]*model.Service {
	chunks := make([][]*model.Service, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(services), chunkCount, chunkSize, counter)
		chunks = append(chunks, services[chunkStart:chunkEnd])
	}

	return chunks
}
