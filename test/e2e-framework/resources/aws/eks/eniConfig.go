// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NewENIConfigs creates ENIConfig CRDs to allow usage of custom networkwing for EKS pods when using AWS VPC CNI Plugin (default).
// https://docs.aws.amazon.com/eks/latest/userguide/cni-custom-network.html
// ENI = Elastic Network Interface (basically, a virtual network card)
func NewENIConfigs(e aws.Environment, subnets []aws.DDInfraEKSPodSubnets, securityGroups pulumi.StringArray, opts ...pulumi.ResourceOption) (*yaml.ConfigGroup, error) {
	if len(subnets) == 0 {
		return nil, fmt.Errorf("subnets must not be empty")
	}

	objects := make([]map[string]interface{}, 0, len(subnets))
	for _, subnet := range subnets {
		objects = append(objects, map[string]interface{}{
			"apiVersion": "crd.k8s.amazonaws.com/v1alpha1",
			"kind":       "ENIConfig",
			"metadata": map[string]interface{}{
				"name": subnet.AZ,
			},
			"spec": map[string]interface{}{
				"securityGroups": securityGroups,
				"subnet":         subnet.SubnetID,
			},
		})
	}

	return yaml.NewConfigGroup(e.Ctx(), e.Namer.ResourceName("eks-eni-configs"), &yaml.ConfigGroupArgs{Objs: objects}, opts...)
}
