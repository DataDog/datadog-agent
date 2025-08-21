// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package loclist

import "testing"

func FuzzParseInstructions(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte, ptrSize uint8, totalByteSize uint32) {
		parsed, err := ParseInstructions(data, ptrSize, totalByteSize)
		if err != nil {
			return
		}
	})
}
