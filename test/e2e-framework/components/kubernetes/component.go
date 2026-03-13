// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
)

// The type that is used to import the KubernetesCluster component
type ClusterOutput struct {
	components.JSONImporter

	ClusterName string `json:"clusterName"`
	KubeConfig  string `json:"kubeConfig"`
}

// Cluster represents a Kubernetes cluster
type Cluster struct {
	pulumi.ResourceState
	components.Component

	KubeProvider *kubernetes.Provider

	ClusterName               pulumi.StringOutput `pulumi:"clusterName"`
	KubeConfig                pulumi.StringOutput `pulumi:"kubeConfig"`
	KubeInternalServerAddress pulumi.StringOutput `pulumi:"kubeInternalServerAddress"`
	KubeInternalServerPort    pulumi.StringOutput `pulumi:"kubeInternalServerPort"`
}

func (c *Cluster) Export(ctx *pulumi.Context, out *ClusterOutput) error {
	return components.Export(ctx, c, out)
}

// Workload is a Component that represents a Kubernetes workload
type Workload struct {
	pulumi.ResourceState
	components.Component
}

type WorkloadAppFunc func(e config.Env, kubeProvider *kubernetes.Provider) (*Workload, error)

// AgentDependentWorkloadAppFunc is a function that deploys a workload app to a kube provider with the agent passed in
type AgentDependentWorkloadAppFunc func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*Workload, error)
