// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package dns

import (
	"math/rand"
	"testing"
)

func TestHostnameFromBytesAllocs(t *testing.T) {
	b := make([]byte, 10)
	s := randomString(b)
	allocs := int(testing.AllocsPerRun(100, func() {
		HostnameFromBytes(s)
	}))
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
