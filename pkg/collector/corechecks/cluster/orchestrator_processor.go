// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package cluster

import (
	"fmt"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsoniter "github.com/json-iterator/go"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func processDeploymentList(deploymentList []*v1.Deployment, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	deployMsgs := make([]*model.Deployment, 0, len(deploymentList))

	for d := 0; d < len(deploymentList); d++ {
		depl := deploymentList[d]
		if orchestrator.SkipKubernetesResource(depl.UID, depl.ResourceVersion, orchestrator.K8sDeployment) {
			continue
		}

		// extract deployment info
		deployModel := extractDeployment(depl)
		// scrub & generate YAML
		if cfg.IsScrubbingEnabled {
			for c := 0; c < len(depl.Spec.Template.Spec.InitContainers); c++ {
				redact.ScrubContainer(&depl.Spec.Template.Spec.InitContainers[c], cfg.Scrubber)
			}
			for c := 0; c < len(deploymentList[d].Spec.Template.Spec.Containers); c++ {
				redact.ScrubContainer(&depl.Spec.Template.Spec.Containers[c], cfg.Scrubber)
			}
		}
		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonDeploy, err := jsoniter.Marshal(depl)
		if err != nil {
			log.Warnf("Could not marshal deployment to JSON: %s", err)
			continue
		}
		deployModel.Yaml = jsonDeploy

		deployMsgs = append(deployMsgs, deployModel)
	}

	groupSize := len(deployMsgs) / cfg.MaxPerMessage
	if len(deployMsgs)%cfg.MaxPerMessage != 0 {
		groupSize++
	}
	chunked := chunkDeployments(deployMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorDeployment{
			ClusterName: cfg.KubeClusterName,
			Deployments: chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d deployments in %s", len(deployMsgs), len(deploymentList), time.Now().Sub(start))
	return messages, nil
}

// chunkDeployments formats and chunks the deployments into a slice of chunks using a specific number of chunks.
func chunkDeployments(deploys []*model.Deployment, chunkCount, chunkSize int) [][]*model.Deployment {
	chunks := make([][]*model.Deployment, 0, chunkCount)

	for c := 1; c <= chunkCount; c++ {
		var (
			chunkStart = chunkSize * (c - 1)
			chunkEnd   = chunkSize * (c)
		)
		// last chunk may be smaller than the chunk size
		if c == chunkCount {
			chunkEnd = len(deploys)
		}
		chunks = append(chunks, deploys[chunkStart:chunkEnd])
	}

	return chunks
}

func processReplicaSetList(rsList []*v1.ReplicaSet, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	rsMsgs := make([]*model.ReplicaSet, 0, len(rsList))

	for rs := 0; rs < len(rsList); rs++ {
		r := rsList[rs]
		if orchestrator.SkipKubernetesResource(r.UID, r.ResourceVersion, orchestrator.K8sReplicaSet) {
			continue
		}

		// extract replica set info
		rsModel := extractReplicaSet(r)

		// scrub & generate YAML
		if cfg.IsScrubbingEnabled {
			for c := 0; c < len(r.Spec.Template.Spec.InitContainers); c++ {
				redact.ScrubContainer(&r.Spec.Template.Spec.InitContainers[c], cfg.Scrubber)
			}
			for c := 0; c < len(r.Spec.Template.Spec.Containers); c++ {
				redact.ScrubContainer(&r.Spec.Template.Spec.Containers[c], cfg.Scrubber)
			}
		}

		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonRS, err := jsoniter.Marshal(r)
		if err != nil {
			log.Warnf("Could not marshal replica set to JSON: %s", err)
			continue
		}
		rsModel.Yaml = jsonRS

		rsMsgs = append(rsMsgs, rsModel)
	}

	groupSize := len(rsMsgs) / cfg.MaxPerMessage
	if len(rsMsgs)%cfg.MaxPerMessage != 0 {
		groupSize++
	}
	chunked := chunkReplicaSets(rsMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorReplicaSet{
			ClusterName: cfg.KubeClusterName,
			ReplicaSets: chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d replica sets in %s", len(rsMsgs), len(rsList), time.Now().Sub(start))
	return messages, nil
}

// chunkReplicaSets formats and chunks the replica sets into a slice of chunks using a specific number of chunks.
func chunkReplicaSets(replicaSets []*model.ReplicaSet, chunkCount, chunkSize int) [][]*model.ReplicaSet {
	chunks := make([][]*model.ReplicaSet, 0, chunkCount)

	for c := 1; c <= chunkCount; c++ {
		var (
			chunkStart = chunkSize * (c - 1)
			chunkEnd   = chunkSize * (c)
		)
		// last chunk may be smaller than the chunk size
		if c == chunkCount {
			chunkEnd = len(replicaSets)
		}
		chunks = append(chunks, replicaSets[chunkStart:chunkEnd])
	}

	return chunks
}

// processServiceList process a service list into process messages.
func processServiceList(serviceList []*corev1.Service, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	serviceMsgs := make([]*model.Service, 0, len(serviceList))

	for s := 0; s < len(serviceList); s++ {
		svc := serviceList[s]
		if orchestrator.SkipKubernetesResource(svc.UID, svc.ResourceVersion, orchestrator.K8sService) {
			continue
		}

		serviceModel := extractService(svc)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonSvc, err := jsoniter.Marshal(svc)
		if err != nil {
			log.Warnf("Could not marshal service to JSON: %s", err)
			continue
		}
		serviceModel.Yaml = jsonSvc

		serviceMsgs = append(serviceMsgs, serviceModel)
	}

	groupSize := len(serviceMsgs) / cfg.MaxPerMessage
	if len(serviceMsgs)%cfg.MaxPerMessage > 0 {
		groupSize++
	}

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

	log.Debugf("Collected & enriched %d out of %d services in %s", len(serviceMsgs), len(serviceList), time.Now().Sub(start))
	return messages, nil
}

// chunkServices chunks the given list of services, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkServices(services []*model.Service, chunkCount, chunkSize int) [][]*model.Service {
	chunks := make([][]*model.Service, 0, chunkCount)

	for c := 1; c <= chunkCount; c++ {
		var (
			chunkStart = chunkSize * (c - 1)
			chunkEnd   = chunkSize * (c)
		)
		// last chunk may be smaller than the chunk size
		if c == chunkCount {
			chunkEnd = len(services)
		}
		chunks = append(chunks, services[chunkStart:chunkEnd])
	}

	return chunks
}

// processNodesList process a nodes list into process messages.
func processNodesList(nodesList []*corev1.Node, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	nodeMsgs := make([]*model.Node, 0, len(nodesList))

	for s := 0; s < len(nodesList); s++ {
		node := nodesList[s]
		if orchestrator.SkipKubernetesResource(node.UID, node.ResourceVersion, orchestrator.K8sNode) {
			continue
		}

		nodeModel := extractNode(node)
		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonNode, err := jsoniter.Marshal(node)
		if err != nil {
			log.Warnf("Could not marshal node to JSON: %s", err)
			continue
		}
		nodeModel.Yaml = jsonNode

		// additional tags
		for _, tag := range convertNodeStatusToTags(nodeModel.Status.Status) {
			nodeModel.Tags = append(nodeModel.Tags, tag)
		}

		for _, role := range nodeModel.Roles {
			nodeModel.Tags = append(nodeModel.Tags, fmt.Sprintf("%s:%s", kubernetes.KubeNodeRoleTagName, strings.ToLower(role)))
		}

		nodeMsgs = append(nodeMsgs, nodeModel)
	}

	groupSize := len(nodeMsgs) / cfg.MaxPerMessage
	if len(nodeMsgs)%cfg.MaxPerMessage > 0 {
		groupSize++
	}

	chunks := chunkNodes(nodeMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorNode{
			ClusterName: cfg.KubeClusterName,
			ClusterId:   clusterID,
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			Nodes:       chunks[i],
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d nodes in %s", len(nodeMsgs), len(nodesList), time.Now().Sub(start))
	return messages, nil
}

// chunkNodes chunks the given list of nodes, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkNodes(nodes []*model.Node, chunkCount, chunkSize int) [][]*model.Node {
	chunks := make([][]*model.Node, 0, chunkCount)

	for c := 1; c <= chunkCount; c++ {
		var (
			chunkStart = chunkSize * (c - 1)
			chunkEnd   = chunkSize * (c)
		)
		// last chunk may be smaller than the chunk size
		if c == chunkCount {
			chunkEnd = len(nodes)
		}
		chunks = append(chunks, nodes[chunkStart:chunkEnd])
	}

	return chunks
}
