// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package operator

import (
	compkubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
)

// OperatorOutput is used to import the Operator component
type OperatorOutput struct { // nolint:revive, We want to keep the name as <Component>Output
	components.JSONImporter

	Operator compkubernetes.KubernetesObjRefOutput `json:"operator"`
}

// Operator represents an Operator installation
type Operator struct {
	pulumi.ResourceState
	components.Component

	Operator *compkubernetes.KubernetesObjectRef `pulumi:"operator"`
}

func (o *Operator) Export(ctx *pulumi.Context, out *OperatorOutput) error {
	return components.Export(ctx, o, out)
}
