// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// FakeintakeOutput moved to the Pulumi-free components/outputs package.
// DefaultRCSigningKeySeed and RCRootJSON moved with it (single definition).
type FakeintakeOutput = outputs.FakeintakeOutput

type Fakeintake struct {
	pulumi.ResourceState
	components.Component

	Host   pulumi.StringOutput `pulumi:"host"`
	Scheme pulumi.StringOutput `pulumi:"scheme"` // Scheme is a string as it's known in code and is useful to check HTTP/HTTPS
	Port   pulumi.IntOutput    `pulumi:"port"`   // Same for Port

	URL pulumi.StringOutput `pulumi:"url"`
}

func (fi *Fakeintake) Export(ctx *pulumi.Context, out *FakeintakeOutput) error {
	return components.Export(ctx, fi, out)
}

// DefaultRCSigningKeySeed is the fixed ed25519 seed shared by every fakeintake instance.
const DefaultRCSigningKeySeed = outputs.DefaultRCSigningKeySeed

// RCRootJSON computes the TUF root JSON for the default fakeintake RC signing key.
func RCRootJSON() (string, error) { return outputs.RCRootJSON() }
