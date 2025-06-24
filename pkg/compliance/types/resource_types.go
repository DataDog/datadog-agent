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
