package ecs

import (
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Workload struct {
	pulumi.ResourceState
	components.Component
}
