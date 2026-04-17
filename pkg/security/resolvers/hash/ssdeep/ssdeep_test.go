// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ssdeep

import (
	"crypto/rand"
	"fmt"
	"testing"

	glaslos "github.com/glaslos/ssdeep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateData(size int, pattern byte) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = pattern + byte(i%251)
	}
	return data
}

func generateRandomData(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

// TestCompatibility verifies identical output vs glaslos/ssdeep for various inputs.
func TestCompatibility(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"8KB_pattern_a", generateData(8*1024, 'a')},
		{"8KB_pattern_z", generateData(8*1024, 'z')},
		{"64KB_pattern", generateData(64*1024, 0x00)},
		{"256KB_pattern", generateData(256*1024, 0x42)},
		{"1MB_pattern", generateData(1024*1024, 0x7f)},
		{"5MB_pattern", generateData(5*1024*1024, 0xAB)},
	}
	for _, size := range []int{8192, 65536, 262144, 1 << 20} {
		data := generateRandomData(size)
		tests = append(tests, struct {
			name string
			data []byte
		}{fmt.Sprintf("%dB_random", size), data})
	}

	glaslos.Force = true
	Force = true
	defer func() {
		glaslos.Force = false
		Force = false
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, err := glaslos.FuzzyBytes(tt.data)
			require.NoError(t, err)

			h := New()
			h.Write(tt.data)
			got := string(h.Sum(nil))

			assert.Equal(t, expected, got,
				"hash mismatch: glaslos=%q cgo=%q", expected, got)
		})
	}
}

// TestChunkedWrite verifies that writing in chunks produces the same result.
func TestChunkedWrite(t *testing.T) {
	glaslos.Force = true
	Force = true
	defer func() {
		glaslos.Force = false
		Force = false
	}()

	data := generateRandomData(256 * 1024)

	expected, err := glaslos.FuzzyBytes(data)
	require.NoError(t, err)

	h := New()
	for i := 0; i < len(data); i += 4096 {
		end := i + 4096
		if end > len(data) {
			end = len(data)
		}
		h.Write(data[i:end])
	}
	got := string(h.Sum(nil))
	assert.Equal(t, expected, got)
}

// TestFileTooSmall verifies the Force flag behavior for small inputs.
func TestFileTooSmall(t *testing.T) {
	data := generateData(2048, 'x')

	h := New()
	h.Write(data)

	Force = false
	result := h.Sum(nil)
	assert.Nil(t, result)
	assert.ErrorIs(t, h.Err(), ErrFileTooSmall)

	Force = true
	defer func() { Force = false }()
	h.Reset()
	h.Write(data)
	result = h.Sum(nil)
	assert.NotEmpty(t, result)
	assert.NoError(t, h.Err())
}

// TestEmptyWrite verifies that empty writes are handled correctly.
func TestEmptyWrite(t *testing.T) {
	h := New()
	n, err := h.Write(nil)
	assert.Equal(t, 0, n)
	assert.NoError(t, err)

	n, err = h.Write([]byte{})
	assert.Equal(t, 0, n)
	assert.NoError(t, err)
}

// TestReset verifies that Reset clears state for reuse.
func TestReset(t *testing.T) {
	Force = true
	defer func() { Force = false }()

	data := generateRandomData(8192)

	h := New()
	h.Write(data)
	first := string(h.Sum(nil))

	h.Reset()
	h.Write(data)
	second := string(h.Sum(nil))

	assert.Equal(t, first, second)
}

var benchSizes = []struct {
	name string
	size int
}{
	{"8KB", 8 * 1024},
	{"64KB", 64 * 1024},
	{"256KB", 256 * 1024},
	{"1MB", 1024 * 1024},
	{"5MB", 5 * 1024 * 1024},
}

func BenchmarkGlaslos(b *testing.B) {
	glaslos.Force = true
	defer func() { glaslos.Force = false }()

	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			for b.Loop() {
				h := glaslos.New()
				h.Write(data)
				h.Sum(nil)
			}
		})
	}
}

func BenchmarkCGO(b *testing.B) {
	Force = true
	defer func() { Force = false }()

	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			for b.Loop() {
				h := New()
				h.Write(data)
				h.Sum(nil)
			}
		})
	}
}
