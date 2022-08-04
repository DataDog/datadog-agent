// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package utils

import (
	"math/rand"
	"runtime"
	"testing"
	"time"
)

func BenchmarkNetNSPathFromPid(b *testing.B) {
	rand.Seed(time.Now().UnixNano())
	pid := rand.Uint32()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := NetNSPathFromPid(pid)
		runtime.KeepAlive(s)
	}
}
