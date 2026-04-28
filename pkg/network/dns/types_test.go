// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package dns

import (
	"math/rand"
	"runtime"
	"testing"
)

func TestHostnameFromBytesAllocs(t *testing.T) {
	b := make([]byte, 10)
	s := randomString(b)
	// Pre-intern the value and hold a reference so the GC cannot collect it
	// between AllocsPerRun iterations.
	keep := HostnameFromBytes(s)
	allocs := int(testing.AllocsPerRun(100, func() {
		HostnameFromBytes(s)
	}))
	runtime.KeepAlive(keep)
	if allocs != 0 {
		t.Errorf("HostnameFromBytes allocated %d objects, want 0", allocs)
	}
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randomString(b []byte) []byte {
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return b
}
