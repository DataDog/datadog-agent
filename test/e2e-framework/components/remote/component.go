// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// HostOutput moved to the Pulumi-free components/outputs package.
type HostOutput = outputs.HostOutput

// Host represents a remote host (for instance, a VM)
type Host struct {
	pulumi.ResourceState
	components.Component

	OS os.OS

	Address       pulumi.StringOutput `pulumi:"address"`
	Port          pulumi.IntOutput    `pulumi:"port"`
	Username      pulumi.StringOutput `pulumi:"username"`
	Password      pulumi.StringOutput `pulumi:"password"`
	Architecture  pulumi.StringOutput `pulumi:"architecture"`
	OSFamily      pulumi.IntOutput    `pulumi:"osFamily"`
	OSFlavor      pulumi.IntOutput    `pulumi:"osFlavor"`
	OSVersion     pulumi.StringOutput `pulumi:"osVersion"`
	CloudProvider pulumi.StringOutput `pulumi:"cloudProvider"`

	PoolInstanceID      pulumi.StringOutput `pulumi:"poolInstanceId"`
	PoolLeaseToken      pulumi.StringOutput `pulumi:"poolLeaseToken"`
	PoolRegion          pulumi.StringOutput `pulumi:"poolRegion"`
	PoolProfile         pulumi.StringOutput `pulumi:"poolProfile"`
	PoolLeaseBucket     pulumi.StringOutput `pulumi:"poolLeaseBucket"`
	PoolBaselineImageID pulumi.StringOutput `pulumi:"poolBaselineImageId"`
	PoolStackID         pulumi.StringOutput `pulumi:"poolStackId"`
}

func (h *Host) Export(ctx *pulumi.Context, out *HostOutput) error {
	return components.Export(ctx, h, out)
}
