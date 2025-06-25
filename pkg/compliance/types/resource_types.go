// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines some shared types used in the compliance system.
package types

// ResourceType represents the type of a resource in the compliance system.
type ResourceType string

const (
	// ResourceTypeHostAPTConfig is used to represent the APT configuration on a host.
	ResourceTypeHostAPTConfig ResourceType = "host_apt_config"
	// ResourceTypeDbCassandra is used to represent a Cassandra database.
	ResourceTypeDbCassandra ResourceType = "db_cassandra"
	// ResourceTypeDbMongodb is used to represent a MongoDB database.
	ResourceTypeDbMongodb ResourceType = "db_mongodb"
	// ResourceTypeDbPostgresql is used to represent a PostgreSQL database.
	ResourceTypeDbPostgresql ResourceType = "db_postgresql"
	// ResourceTypeAwsEksWorkerNode is used to represent an EKS worker node.
	ResourceTypeAwsEksWorkerNode ResourceType = "aws_eks_worker_node"
	// ResourceTypeAzureAksWorkerNode is used to represent an AKS worker node.
	ResourceTypeAzureAksWorkerNode ResourceType = "azure_aks_worker_node"
	// ResourceTypeGcpGkeWorkerNode is used to represent a GKE worker node.
	ResourceTypeGcpGkeWorkerNode ResourceType = "gcp_gke_worker_node"
	// ResourceTypeKubernetesMasterNode is used to represent a Kubernetes master node.
	ResourceTypeKubernetesMasterNode ResourceType = "kubernetes_master_node"
	// ResourceTypeKubernetesWorkerNode is used to represent a Kubernetes worker node.
	ResourceTypeKubernetesWorkerNode ResourceType = "kubernetes_worker_node"
)
