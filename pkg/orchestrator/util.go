// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package orchestrator

import "github.com/DataDog/datadog-agent/pkg/util/log"

// NodeType represents a kind of resource used by a container orchestrator.
type NodeType int

const (
	// K8sDeployment represents a Kubernetes Deployment
	K8sDeployment NodeType = iota
	// K8sPod represents a Kubernetes Pod
	K8sPod
	// K8sReplicaSet represents a Kubernetes ReplicaSet
	K8sReplicaSet
	// K8sService represents a Kubernetes Service
	K8sService
	// K8sNode represents a Kubernetes Node
	K8sNode
)

// NodeTypes returns the current existing NodesTypes as a slice to iterate over.
func NodeTypes() []NodeType {
	return []NodeType{K8sNode, K8sPod, K8sReplicaSet, K8sDeployment, K8sService}
}

func (n NodeType) String() string {
	switch n {
	case K8sNode:
		return "Node"
	case K8sService:
		return "Service"
	case K8sReplicaSet:
		return "ReplicaSet"
	case K8sDeployment:
		return "Deployment"
	case K8sPod:
		return "Pod"
	default:
		log.Errorf("trying to convert unknown NodeType iota: %v", n)
		return ""
	}
}
