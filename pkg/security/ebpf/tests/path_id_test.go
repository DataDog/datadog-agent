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

func TestPathID(t *testing.T) {
	t.Run("mount-bump", func(t *testing.T) {
		var ctx baloum.StdContext
		code, err := newVM(t).RunProgram(&ctx, "test/path_id_mount_and_invalidation")
		if err != nil || code != 1 {
			t.Errorf("unexpected error: %v, %d", err, code)
		}
	})

	t.Run("link-bump", func(t *testing.T) {
		var ctx baloum.StdContext
		code, err := newVM(t).RunProgram(&ctx, "test/path_id_link_and_invalidation")
		if err != nil || code != 1 {
			t.Errorf("unexpected error: %v, %d", err, code)
		}
	})

	t.Run("rename-bump", func(t *testing.T) {
		var ctx baloum.StdContext
		code, err := newVM(t).RunProgram(&ctx, "test/path_id_rename_and_invalidation")
		if err != nil || code != 1 {
			t.Errorf("unexpected error: %v, %d", err, code)
		}
	})
}
