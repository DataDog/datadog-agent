// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package utils

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(2*time.Second, 2)

	if !rl.Allow(uint64(time.Now().UnixNano())) {
		t.Error("should be allowed")
	}

	if !rl.Allow(uint64(time.Now().UnixNano())) {
		t.Error("should be allowed")
	}

	if rl.Allow(uint64(time.Now().UnixNano())) {
		t.Error("shouldn't be allowed")
	}

	time.Sleep(3 * time.Second)

	if !rl.Allow(uint64(time.Now().UnixNano())) {
		t.Error("should be allowed")
	}

	if !rl.Allow(uint64(time.Now().UnixNano())) {
		t.Error("should be allowed")
	}

	if rl.Allow(uint64(time.Now().UnixNano())) {
		t.Error("shouldn't be allowed")
	}
}
