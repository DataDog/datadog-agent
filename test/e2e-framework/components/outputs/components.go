// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package outputs

import (
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
)

// HostOutput is the type that is used to import the Host component.
type HostOutput struct {
	JSONImporter

	CloudProvider CloudProviderIdentifier `json:"cloudProvider"`

	Address      string          `json:"address"`
	Port         int             `json:"port"`
	Username     string          `json:"username"`
	Password     string          `json:"password,omitempty"`
	OSFamily     e2eos.Family    `json:"osFamily"`
	OSFlavor     e2eos.Flavor    `json:"osFlavor"`
	OSVersion    string          `json:"osVersion"`
	Architecture e2eos.Architecture `json:"architecture"`

	// Pool* are set only when the host is a macOS EC2 pool member (see
	// resources/aws/ec2/pool). BaseSuite reads them at teardown to revert and release
	// the instance. An empty PoolLeaseToken with a set PoolInstanceID means the member
	// was just created and still needs its first lease published.
	PoolInstanceID      string `json:"poolInstanceId,omitempty"`
	PoolLeaseToken      string `json:"poolLeaseToken,omitempty"`
	PoolRegion         string `json:"poolRegion,omitempty"`
	PoolProfile        string `json:"poolProfile,omitempty"`
	PoolLeaseBucket    string `json:"poolLeaseBucket,omitempty"`
	PoolBaselineImageID string `json:"poolBaselineImageId,omitempty"`
	PoolStackID        string `json:"poolStackId,omitempty"`
}

// ClusterOutput is the type that is used to import the KubernetesCluster component.
type ClusterOutput struct {
	JSONImporter

	ClusterName string `json:"clusterName"`
	KubeConfig  string `json:"kubeConfig"`
}

// KubernetesObjRefOutput describes a Kubernetes object by reference.
type KubernetesObjRefOutput struct { // nolint:revive, We want to keep the name as <Component>ObjRefOutput
	JSONImporter

	Namespace      string            `json:"namespace"`
	Name           string            `json:"name"`
	Kind           string            `json:"kind"`
	AppVersion     string            `json:"installAppVersion"`
	Version        string            `json:"installVersion"`
	LabelSelectors map[string]string `json:"labelSelectors"`
}

// KubernetesAgentOutput describes the agent workloads of a Kubernetes environment.
type KubernetesAgentOutput struct {
	JSONImporter

	LinuxNodeAgent     KubernetesObjRefOutput `json:"linuxNodeAgent"`
	LinuxClusterAgent  KubernetesObjRefOutput `json:"linuxClusterAgent"`
	LinuxClusterChecks KubernetesObjRefOutput `json:"linuxClusterChecks"`

	WindowsNodeAgent     KubernetesObjRefOutput `json:"windowsNodeAgent"`
	WindowsClusterAgent  KubernetesObjRefOutput `json:"windowsClusterAgent"`
	WindowsClusterChecks KubernetesObjRefOutput `json:"windowsClusterChecks"`

	FIPSEnabled bool `json:"fipsEnabled"`
}

// HostAgentOutput describes an agent installed on a remote host.
type HostAgentOutput struct {
	JSONImporter

	Host        HostOutput `json:"host"`
	FIPSEnabled bool       `json:"fipsEnabled"`
}

// HostUpdaterOutput describes an updater installed on a remote host.
type HostUpdaterOutput struct {
	JSONImporter
}

// ManagerOutput is the type that is used to import the docker Manager component.
type ManagerOutput struct {
	JSONImporter

	Host HostOutput `json:"host"`
}

// ECSClusterOutput is the type that is used to import the ECS Cluster component.
// (Named with the ECS prefix here to live in the shared outputs package; the
// ecs package aliases it as ecs.ClusterOutput.)
type ECSClusterOutput struct {
	JSONImporter

	ClusterName string `json:"clusterName"`
	ClusterArn  string `json:"clusterArn"`
}

// ActiveDirectoryOutput is the type used to import the Active Directory component.
// (The activedirectory package aliases it as activedirectory.Output.)
type ActiveDirectoryOutput struct {
	JSONImporter
}

// DockerAgentOutput describes an agent installed in a Docker container.
type DockerAgentOutput struct {
	JSONImporter

	DockerManager ManagerOutput `json:"dockerManager"`
	ContainerName string        `json:"containerName"`
	FIPSEnabled   bool          `json:"fipsEnabled"`
}
