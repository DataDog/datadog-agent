// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && ebpf_bindata

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/safchain/baloum/pkg/baloum"
)

func TestCredentialSource(t *testing.T) {
	for _, family := range []string{"ipv4", "ipv6"} {
		t.Run(family, func(t *testing.T) {
			var ctx baloum.StdContext
			code, err := newVM(t).RunProgram(&ctx, "test/credential_source_"+family)
			if err != nil || code != 1 {
				t.Fatalf("unexpected error: %v, return code: %d", err, code)
			}
		})
	}
}
