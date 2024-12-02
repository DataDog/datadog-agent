// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/test-infra-definitions/components/ecs"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"

	clientecs "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/ecs"
)

// ECSCluster is an ECS Cluster
type ECSCluster struct {
	ecs.ClusterOutput

	ECSClient *clientecs.Client
}

var _ e2e.Initializable = &ECSCluster{}

// Init is called by e2e test Suite after the component is provisioned.
func (c *ECSCluster) Init(e2e.Context) error {

	ecsClient, err := clientecs.NewClient(c.ClusterOutput.ClusterName)
	if err != nil {
		return err
	}

	c.ECSClient = ecsClient

	return nil
}
