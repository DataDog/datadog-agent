// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// The type that is used to import the ECS Cluster component
type ClusterOutput struct {
	components.JSONImporter

	ClusterName string `json:"clusterName"`
	ClusterArn  string `json:"clusterArn"`
}

// Cluster represents a ECS cluster
type Cluster struct {
	pulumi.ResourceState
	components.Component

	ClusterName pulumi.StringOutput `pulumi:"clusterName"`
	ClusterArn  pulumi.StringOutput `pulumi:"clusterArn"`
}

func (c *Cluster) Export(ctx *pulumi.Context, out *ClusterOutput) error {
	return components.Export(ctx, c, out)
}
