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

func TestActivityDumpRateLimiterBasic(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/ad_ratelimiter_basic")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

func TestActivityDumpRateLimiterBasicHalf(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/ad_ratelimiter_basic_half")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

func TestActivityDumpRateLimiterDecreasingDroprate(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/ad_ratelimiter_decreasing_droprate")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

func TestActivityDumpRateLimiterIncreasingDroprate(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/ad_ratelimiter_increasing_droprate")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}
