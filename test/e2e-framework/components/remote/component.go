// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// HostOutput is the type that is used to import the Host component
type HostOutput struct {
	components.JSONImporter

	CloudProvider components.CloudProviderIdentifier `json:"cloudProvider"`

	Address      string          `json:"address"`
	Port         int             `json:"port"`
	Username     string          `json:"username"`
	Password     string          `json:"password,omitempty"`
	OSFamily     os.Family       `json:"osFamily"`
	OSFlavor     os.Flavor       `json:"osFlavor"`
	OSVersion    string          `json:"osVersion"`
	Architecture os.Architecture `json:"architecture"`

	// Pool* are set only when the host is a macOS EC2 pool member (see
	// resources/aws/ec2/pool). BaseSuite reads them at teardown to revert and release
	// the instance. An empty PoolLeaseToken with a set PoolInstanceID means the member
	// was just created and still needs its first lease published.
	PoolInstanceID      string `json:"poolInstanceId,omitempty"`
	PoolLeaseToken      string `json:"poolLeaseToken,omitempty"`
	PoolRegion          string `json:"poolRegion,omitempty"`
	PoolProfile         string `json:"poolProfile,omitempty"`
	PoolBaselineImageID string `json:"poolBaselineImageId,omitempty"`
	PoolStackID         string `json:"poolStackId,omitempty"`
}

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
	PoolBaselineImageID pulumi.StringOutput `pulumi:"poolBaselineImageId"`
	PoolStackID         pulumi.StringOutput `pulumi:"poolStackId"`
}

func (h *Host) Export(ctx *pulumi.Context, out *HostOutput) error {
	return components.Export(ctx, h, out)
}
