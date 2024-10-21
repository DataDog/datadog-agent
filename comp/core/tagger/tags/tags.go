// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags contains the list of tags that are added by the tagger
package tags

const (
	// STANDARD TAGS

	// Env is the standard tag for the environment
	Env = "env"
	// Version is the standard tag for the version
	Version = "version"
	// Service is the standard tag for the service
	Service = "service"

	// LOW CARDINALITY

	// ClusterName is the tag for the cluster name
	ClusterName = "cluster_name"

	// ImageName is the tag for the image name
	ImageName = "image_name"
	// ShortImage is the tag for the short image name
	ShortImage = "short_image"
	// ImageTag is the tag for the image tag
	ImageTag = "image_tag"
	// ImageID is the tag for the image ID
	ImageID = "image_id"
	// DockerImage is the tag for the docker image
	DockerImage = "docker_image"
	// OSName is the tag for the OS name of the container image
	OSName = "os_name"
	// OSVersion is the tag for the OS version of the container image
	OSVersion = "os_version"
	// Architecture is the tag for the architecture of the container image
	Architecture = "architecture"

	// PodPhase is the tag for the pod phase
	PodPhase = "pod_phase"
	// DisplayContainerName is the tag for the display container name
	DisplayContainerName = "display_container_name"
	// KubePriorityClass is the tag for the Kubernetes priority class
	KubePriorityClass = "kube_priority_class"
	// KubeQOS is the tag for the Kubernetes QoS (Quality of Service)
	KubeQOS = "kube_qos"
	// KubeRuntimeClass is the tag for the Kubernetes runtime class
	KubeRuntimeClass = "kube_runtime_class"
	// KubeContainerName is the tag for the Kubernetes container name
	KubeContainerName = "kube_container_name"
	// KubeOwnerRefKind is the tag for the Kubernetes owner reference kind
	KubeOwnerRefKind = "kube_ownerref_kind"

	// KubePod is the tag for the pod name
	KubePod = "pod_name"
	// KubeDeployment is the tag for the deployment name
	KubeDeployment = "kube_deployment"
	// KubeReplicaSet is the tag for the replica set name
	KubeReplicaSet = "kube_replica_set"
	// KubeReplicationController is the tag for the replication controller name
	KubeReplicationController = "kube_replication_controller"
	// KubeStatefulSet is the tag for the stateful set name
	KubeStatefulSet = "kube_stateful_set"
	// KubeDaemonSet is the tag for the daemon set name
	KubeDaemonSet = "kube_daemon_set"
	// KubeJob is the tag for the job name
	KubeJob = "kube_job"
	// KubeCronjob is the tag for the cronjob name
	KubeCronjob = "kube_cronjob"
	// KubeService is the tag for the service name
	KubeService = "kube_service"
	// KubeNamespace is the tag for the namespace name
	KubeNamespace = "kube_namespace"
	// KubePersistentVolumeClaim is the tag for the persistent volume name
	KubePersistentVolumeClaim = "persistentvolumeclaim"

	// KubeAppName is the tag for the "app.kubernetes.io/name" Kubernetes label
	KubeAppName = "kube_app_name"
	// KubeAppInstance is the tag for the "app.kubernetes.io/instance" Kubernetes label
	KubeAppInstance = "kube_app_instance"
	// KubeAppVersion is the tag for the "app.kubernetes.io/version" Kubernetes label
	KubeAppVersion = "kube_app_version"
	// KubeAppComponent is the tag for the "app.kubernetes.io/component" Kubernetes label
	KubeAppComponent = "kube_app_component"
	// KubeAppPartOf is the tag for the "app.kubernetes.io/part-of" Kubernetes label
	KubeAppPartOf = "kube_app_part_of"
	// KubeAppManagedBy is the tag for the "app.kubernetes.io/managed-by" Kubernetes label
	KubeAppManagedBy = "kube_app_managed_by"

	// GPU related tags

	// KubeGPUVendor the tag for the Kubernetes Resource GPU vendor
	KubeGPUVendor = "gpu_vendor"

	// OpenshiftDeploymentConfig is the tag for the OpenShift deployment config name
	OpenshiftDeploymentConfig = "oshift_deployment_config"

	// TaskName is the tag for the ECS task name
	TaskName = "task_name"
	// TaskFamily is the tag for the ECS task family
	TaskFamily = "task_family"
	// TaskVersion is the tag for the ECS task version
	TaskVersion = "task_version"
	// Region is the tag for the ECS region
	Region = "region"
	// AvailabilityZone is the tag for the ECS availability zone
	AvailabilityZone = "availability-zone"
	// AvailabilityZoneDeprecated is the tag for the ECS availability zone (deprecated)
	AvailabilityZoneDeprecated = "availability_zone"
	// EcsContainerName is the tag for the ECS container name
	EcsContainerName = "ecs_container_name"
	// EcsClusterName is the tag for the ECS cluster name
	EcsClusterName = "ecs_cluster_name"
	// EcsServiceName is the tag for the ECS service name
	EcsServiceName = "ecs_service"
	// AwsAccount is the tag for ECS account id
	AwsAccount = "aws_account"

	// Language is the tag for the process language
	Language = "language"

	// MarathonApp is the tag for the Marathon app ID
	MarathonApp = "marathon_app"

	// ChronosJob is the tag for the Chronos job
	ChronosJob = "chronos_job"
	// ChronosJobOwner is the tag for the Chronos job owner
	ChronosJobOwner = "chronos_job_owner"

	// NomadTask is the tag for the Nomad task
	NomadTask = "nomad_task"
	// NomadJob is the tag for the Nomad job
	NomadJob = "nomad_job"
	// NomadGroup is the tag for the Nomad group
	NomadGroup = "nomad_group"
	// NomadNamespace is the tag for the Nomad namespace
	NomadNamespace = "nomad_namespace"
	// NomadDC is the tag for the Nomad datacenter
	NomadDC = "nomad_dc"

	// SwarmService is the tag for the Docker Swarm service
	SwarmService = "swarm_service"
	// SwarmNamespace is the tag for the Docker Swarm namespace
	SwarmNamespace = "swarm_namespace"

	// RancherStack is the tag for the Rancher stack
	RancherStack = "rancher_stack"
	// RancherService is the tag for the Rancher service
	RancherService = "rancher_service"

	// GitCommitSha is the tag for the Git commit SHA
	GitCommitSha = "git.commit.sha"
	// GitRepository is the tag for the Git repository URL
	GitRepository = "git.repository_url"

	// RemoteConfigID is the tag for the remote config ID
	RemoteConfigID = "dd_remote_config_id"
	// RemoteConfigRevision is the tag for the remote config revision
	RemoteConfigRevision = "dd_remote_config_rev"

	// ORCHESTRATOR CARDINALITY

	// KubeOwnerRefName is the tag for the Kubernetes owner reference name
	KubeOwnerRefName = "kube_ownerref_name"
	// OpenshiftDeployment is the tag for the OpenShift deployment name
	OpenshiftDeployment = "oshift_deployment"
	// TaskARN is the tag for the task ARN (Amazon Resource Name)
	TaskARN = "task_arn"
	// MesosTask is the tag for the Mesos task
	MesosTask = "mesos_task"

	// HIGH CARDINALITY

	// ContainerName is the tag for the container name
	ContainerName = "container_name"
	// ContainerID is the tag for the container ID
	ContainerID = "container_id"
	// RancherContainer is the tag for the Rancher container name
	RancherContainer = "rancher_container"
)
