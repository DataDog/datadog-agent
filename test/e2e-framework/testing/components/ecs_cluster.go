// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	clientecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/ecs"
)

// ECSCluster is an ECS Cluster
type ECSCluster struct {
	ecs.ClusterOutput

	ECSClient *clientecs.Client
}

var _ common.Initializable = &ECSCluster{}

// Init is called by e2e test Suite after the component is provisioned.
func (c *ECSCluster) Init(common.Context) error {

	ecsClient, err := clientecs.NewClient(c.ClusterOutput.ClusterName)
	if err != nil {
		return err
	}

	c.ECSClient = ecsClient

	return nil
}
