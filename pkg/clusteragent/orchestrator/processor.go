// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsoniter "github.com/json-iterator/go"
	yaml "gopkg.in/yaml.v2"
	v1 "k8s.io/api/apps/v1"
)

func processDeploymentList(deploymentList []*v1.Deployment, groupID int32, cfg *config.AgentConfig, clusterName string, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	deployMsgs := make([]*model.Deployment, 0, len(deploymentList))

	for d := 0; d < len(deploymentList); d++ {
		// extract deployment info
		deployModel := extractDeployment(deploymentList[d])

		// k8s objects only have json "omitempty" annotations
		// we're doing json<>yaml to get rid of the null properties
		jsonDeploy, err := jsoniter.Marshal(deploymentList[d])
		if err != nil {
			log.Debugf("Could not marshal deployment in JSON: %s", err)
			continue
		}
		var jsonObj interface{}
		yaml.Unmarshal(jsonDeploy, &jsonObj)
		yamlDeploy, _ := yaml.Marshal(jsonObj)
		deployModel.Yaml = yamlDeploy

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
			ClusterName: clusterName,
			Deployments: chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
		})
	}

	log.Debugf("Collected & enriched %d deployments in %s", len(deployMsgs), time.Now().Sub(start))
	return messages, nil
}

// chunkDeployments formats and chunks the deployments into a slice of chunks using a specific number of chunks.
func chunkDeployments(deploys []*model.Deployment, chunks, perChunk int) [][]*model.Deployment {
	chunked := make([][]*model.Deployment, 0, chunks)
	chunk := make([]*model.Deployment, 0, perChunk)

	for _, p := range deploys {
		chunk = append(chunk, p)
		if len(chunk) == perChunk {
			chunked = append(chunked, chunk)
			chunk = make([]*model.Deployment, 0, perChunk)
		}
	}
	if len(chunk) > 0 {
		chunked = append(chunked, chunk)
	}
	return chunked
}

func processReplicasetList(rsList []*v1.ReplicaSet, groupID int32, cfg *config.AgentConfig, clusterName string, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	rsMsgs := make([]*model.ReplicaSet, 0, len(rsList))

	for d := 0; d < len(rsList); d++ {
		// extract replica set info
		rsModel := extractReplicaSet(rsList[d])

		// k8s objects only have json "omitempty" annotations
		// we're doing json<>yaml to get rid of the null properties
		jsonDeploy, err := jsoniter.Marshal(rsList[d])
		if err != nil {
			log.Debugf("Could not marshal replica set in JSON: %s", err)
			continue
		}
		var jsonObj interface{}
		yaml.Unmarshal(jsonDeploy, &jsonObj)
		yamlDeploy, _ := yaml.Marshal(jsonObj)
		rsModel.Yaml = yamlDeploy

		rsMsgs = append(rsMsgs, rsModel)
	}

	groupSize := len(rsMsgs) / cfg.MaxPerMessage
	if len(rsMsgs)%cfg.MaxPerMessage != 0 {
		groupSize++
	}
	chunked := chunkReplicasets(rsMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorReplicaSet{
			ClusterName: clusterName,
			Replicasets: chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
		})
	}

	log.Debugf("Collected & enriched %d replica sets in %s", len(rsMsgs), time.Now().Sub(start))
	return messages, nil
}

// chunkReplicasets formats and chunks the replica sets into a slice of chunks using a specific number of chunks.
func chunkReplicasets(deploys []*model.ReplicaSet, chunks, perChunk int) [][]*model.ReplicaSet {
	chunked := make([][]*model.ReplicaSet, 0, chunks)
	chunk := make([]*model.ReplicaSet, 0, perChunk)

	for _, p := range deploys {
		chunk = append(chunk, p)
		if len(chunk) == perChunk {
			chunked = append(chunked, chunk)
			chunk = make([]*model.ReplicaSet, 0, perChunk)
		}
	}
	if len(chunk) > 0 {
		chunked = append(chunked, chunk)
	}
	return chunked
}
