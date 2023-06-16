// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata

package tests

import (
	"testing"

	"github.com/safchain/baloum/pkg/baloum"
)

func TestDiscarderEventMask(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/discarders_event_mask")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

func TestDiscarderRetention(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/discarders_retention")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

func TestDiscarderRevision(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/discarders_revision")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

func TestDiscarderMountRevision(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/discarders_mount_revision")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}
