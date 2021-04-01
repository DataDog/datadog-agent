// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	// K8sCluster represents a Kubernetes Cluster
	K8sCluster
)

var (
	telemetryTags = map[NodeType][]string{
		K8sCluster:    getTelemetryTags(K8sCluster),
		K8sDeployment: getTelemetryTags(K8sDeployment),
		K8sNode:       getTelemetryTags(K8sNode),
		K8sPod:        getTelemetryTags(K8sPod),
		K8sReplicaSet: getTelemetryTags(K8sReplicaSet),
		K8sService:    getTelemetryTags(K8sService),
	}
)

// NodeTypes returns the current existing NodesTypes as a slice to iterate over.
func NodeTypes() []NodeType {
	return []NodeType{
		K8sCluster,
		K8sDeployment,
		K8sNode,
		K8sPod,
		K8sReplicaSet,
		K8sService,
	}
}

func (n NodeType) String() string {
	switch n {
	case K8sCluster:
		return "Cluster"
	case K8sDeployment:
		return "Deployment"
	case K8sNode:
		return "Node"
	case K8sPod:
		return "Pod"
	case K8sReplicaSet:
		return "ReplicaSet"
	case K8sService:
		return "Service"
	default:
		log.Errorf("Trying to convert unknown NodeType iota: %v", n)
		return ""
	}
}

// Orchestrator returns the orchestrator name for a node type.
func (n NodeType) Orchestrator() string {
	switch n {
	case K8sCluster, K8sDeployment, K8sNode, K8sPod, K8sReplicaSet, K8sService:
		return "k8s"
	default:
		log.Errorf("Unknown NodeType %v", n)
		return ""
	}
}

// TelemetryTags return tags used for telemetry.
func (n NodeType) TelemetryTags() []string {
	tags, ok := telemetryTags[n]
	if !ok {
		log.Errorf("Unknown NodeType %v", n)
		return []string{"unknown", "unknown"}
	}
	return tags
}

func getTelemetryTags(n NodeType) []string {
	return []string{
		n.Orchestrator(),
		strings.ToLower(n.String()),
	}
}
