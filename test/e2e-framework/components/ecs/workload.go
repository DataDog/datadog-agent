// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Workload struct {
	pulumi.ResourceState
	components.Component
}
