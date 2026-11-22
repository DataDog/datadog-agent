// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package binversion

import (
	"bytes"
	"errors"
	"testing"
)

func TestFindBuildInfoSkipsUnalignedMagic(t *testing.T) {
	const headerOffset = 64
	data := make([]byte, headerOffset+buildInfoSize)
	copy(data[1:], buildInfoMagic)
	copy(data[headerOffset:], buildInfoMagic)
	data[headerOffset+14] = 8
	data[headerOffset+15] = 2

	header, err := findBuildInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(header, data[headerOffset:]) {
		t.Errorf("got %x, want header at offset %d", header, headerOffset)
	}
}

func TestFindBuildInfoRejectsUnalignedMagic(t *testing.T) {
	data := make([]byte, 1+buildInfoSize)
	copy(data[1:], buildInfoMagic)

	if _, err := findBuildInfo(data); !errors.Is(err, ErrNotGoExe) {
		t.Errorf("got error %v, want %v", err, ErrNotGoExe)
	}
}
