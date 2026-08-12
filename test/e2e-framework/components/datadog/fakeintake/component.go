// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/fakeintake/server/rcstore"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DefaultRCSigningKeySeed is the fixed ed25519 seed shared by every fakeintake
// instance. It is a test-only key — never use it in production. A single
// well-known seed makes the TUF root JSON deterministic so it can be computed
// at provision time without any runtime key negotiation.
const DefaultRCSigningKeySeed = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

type FakeintakeOutput struct { // nolint:revive, We want to keep the name as <Component>Output
	components.JSONImporter

	Host   string `json:"host"`
	Scheme string `json:"scheme"`
	Port   uint32 `json:"port"`
	URL    string `json:"url"`
}

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

// RCRootJSON computes the TUF root JSON for the default fakeintake RC signing key.
// The result is deterministic — it can be computed at provision time so the agent
// config can reference it before fakeintake has started.
func RCRootJSON() (string, error) {
	priv, err := rcstore.KeyFromHexSeed(DefaultRCSigningKeySeed)
	if err != nil {
		return "", fmt.Errorf("rc signing key: %w", err)
	}
	pubHex := rcstore.PublicKeyHex(priv)
	keyID, err := rcstore.ComputeKeyID(pubHex)
	if err != nil {
		return "", fmt.Errorf("rc key id: %w", err)
	}
	rootJSON, err := rcstore.BuildRootJSON(priv, keyID, pubHex)
	if err != nil {
		return "", fmt.Errorf("rc root json: %w", err)
	}
	return string(rootJSON), nil
}
