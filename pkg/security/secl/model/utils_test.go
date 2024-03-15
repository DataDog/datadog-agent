// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalString(t *testing.T) {
	array := []byte{65, 66, 67, 0, 0, 0, 65, 66}
	str, err := UnmarshalString(array, 8, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "ABC", str)
}

func BenchmarkNullTerminatedString(b *testing.B) {
	array := []byte{65, 66, 67, 0, 0, 0, 65, 66}
	var s string
	for i := 0; i < b.N; i++ {
		s = NullTerminatedString(array, nil)
	}
	runtime.KeepAlive(s)
}
