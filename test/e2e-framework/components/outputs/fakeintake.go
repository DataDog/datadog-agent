// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package outputs

import (
	"github.com/DataDog/datadog-agent/test/fakeintake/server/rcstore"
)

// DefaultRCSigningKeySeed is the fixed ed25519 seed shared by every fakeintake
// instance. It is a test-only key — never use it in production. A single
// well-known seed makes the TUF root JSON deterministic so it can be computed
// at provision time without any runtime key negotiation.
const DefaultRCSigningKeySeed = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// FakeintakeOutput is the type that is used to import the Fakeintake component.
type FakeintakeOutput struct { // nolint:revive, We want to keep the name as <Component>Output
	JSONImporter

	Host   string `json:"host"`
	Scheme string `json:"scheme"`
	Port   uint32 `json:"port"`
	URL    string `json:"url"`
}

// ImageURL stays in the fakeintake Pulumi package (it depends on the runner
// profile, which would form an import cycle from here).

// RCRootJSON returns the Remote Configuration TUF root JSON matching
// DefaultRCSigningKeySeed, so a fakeintake started with --rc-key-data=<seed>
// can be used by an agent installed with this root.
func RCRootJSON() (string, error) {
	priv, err := rcstore.KeyFromHexSeed(DefaultRCSigningKeySeed)
	if err != nil {
		return "", err
	}
	pubHex := rcstore.PublicKeyHex(priv)
	keyID, err := rcstore.ComputeKeyID(pubHex)
	if err != nil {
		return "", err
	}
	rootJSON, err := rcstore.BuildRootJSON(priv, keyID, pubHex)
	if err != nil {
		return "", err
	}
	return string(rootJSON), nil
}
