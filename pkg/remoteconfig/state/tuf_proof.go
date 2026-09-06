// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package state

// ConfigTUFProof contains the Director metadata needed to independently verify
// one Remote Config target from a pinned TUF root.
type ConfigTUFProof struct {
	Roots      [][]byte
	Targets    []byte
	TargetPath string
	TargetFile []byte
}
