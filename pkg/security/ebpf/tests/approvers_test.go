// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/safchain/baloum/pkg/baloum"
)

func TestFlagApprover(t *testing.T) {
	for _, section := range []string{
		"test/flag_approver_low_bits",
		"test/flag_approver_high_bits",
		"test/flag_approver_unset",
	} {
		t.Run(section, func(t *testing.T) {
			var ctx baloum.StdContext
			code, err := newVM(t).RunProgram(&ctx, section)
			if err != nil || code != 1 {
				t.Errorf("unexpected error: %v, %d", err, code)
			}
		})
	}
}
