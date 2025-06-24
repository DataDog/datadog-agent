// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

type ResourceType string

const (
	ResourceTypeHostAPTConfig        ResourceType = "host_apt_config"
	ResourceTypeDbCassandra          ResourceType = "db_cassandra"
	ResourceTypeDbMongodb            ResourceType = "db_mongodb"
	ResourceTypeDbPostgresql         ResourceType = "db_postgresql"
	ResourceTypeAwsEksWorkerNode     ResourceType = "aws_eks_worker_node"
	ResourceTypeAzureAksWorkerNode   ResourceType = "azure_aks_worker_node"
	ResourceTypeGcpGkeWorkerNode     ResourceType = "gcp_gke_worker_node"
	ResourceTypeKubernetesMasterNode ResourceType = "kubernetes_master_node"
	ResourceTypeKubernetesWorkerNode ResourceType = "kubernetes_worker_node"
)
