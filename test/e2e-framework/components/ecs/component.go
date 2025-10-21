package ecs

import (
	"github.com/DataDog/test-infra-definitions/components"
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
