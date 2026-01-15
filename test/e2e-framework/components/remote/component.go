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
}

func (h *Host) Export(ctx *pulumi.Context, out *HostOutput) error {
	return components.Export(ctx, h, out)
}
