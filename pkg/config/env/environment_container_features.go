// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

// Remember to also register feature in init()
const (
	// Docker socket present
	Docker Feature = "docker"
	// Containerd socket present
	Containerd Feature = "containerd"
	// Cri is any cri socket present
	Cri Feature = "cri"
	// Crio socket present
	Crio Feature = "crio"
	// Kubernetes environment
	Kubernetes Feature = "kubernetes"
	// ECSEC2 environment
	ECSEC2 Feature = "ecsec2"
	// ECSFargate environment
	ECSFargate Feature = "ecsfargate"
	// ECSManagedInstances environment
	ECSManagedInstances Feature = "ecsmanagedinstances"
	// EKSFargate environment
	EKSFargate Feature = "eksfargate"
	// KubeOrchestratorExplorer can be enabled
	KubeOrchestratorExplorer Feature = "kube_orchestratorexplorer"
	// KubeletConfigOrchestratorCheck can be enabled
	KubeletConfigOrchestratorCheck = "kubelet_config_orchestrator_check"
	// ECSOrchestratorExplorer can be enabled
	ECSOrchestratorExplorer Feature = "ecs_orchestratorexplorer"
	// CloudFoundry socket present
	CloudFoundry Feature = "cloudfoundry"
	// Podman containers storage path accessible
	Podman Feature = "podman"
	// PodResources socket present
	PodResources Feature = "podresources"
	// NVML library present for GPU detection
	NVML Feature = "nvml"
	// KubernetesDevicePlugins socket directory present and accessible
	KubernetesDevicePlugins Feature = "kubernetes_deviceplugins"
	// NonstandardCRIRuntime is a fallback value for when customers supply a CRI compliant runtime via the
	// cri_socket_path configuration field
	NonstandardCRIRuntime = "nonstandard-cri-runtime"
)
