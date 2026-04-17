// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ssdeep

import (
	"crypto/rand"
	"fmt"
	"os"
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

// TestHashFDCompatibility verifies HashFD produces identical output to glaslos/ssdeep.
func TestHashFDCompatibility(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"8KB_pattern", generateData(8*1024, 'a')},
		{"64KB_pattern", generateData(64*1024, 0x00)},
		{"256KB_random", generateRandomData(256 * 1024)},
		{"1MB_random", generateRandomData(1024 * 1024)},
	}

	glaslos.Force = true
	defer func() { glaslos.Force = false }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, err := glaslos.FuzzyBytes(tt.data)
			require.NoError(t, err)

			f, err := os.CreateTemp("", "ssdeep_test_*")
			require.NoError(t, err)
			defer os.Remove(f.Name())

			_, err = f.Write(tt.data)
			require.NoError(t, err)

			_, err = f.Seek(0, 0)
			require.NoError(t, err)

			got, err := HashFD(f, 0)
			f.Close()
			require.NoError(t, err)
			assert.Equal(t, expected, got,
				"HashFD mismatch: glaslos=%q fd=%q", expected, got)
		})
	}
}

// TestHashFDTooSmall verifies HashFD rejects files under 4096 bytes.
func TestHashFDTooSmall(t *testing.T) {
	f, err := os.CreateTemp("", "ssdeep_small_*")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.Write(generateData(2048, 'x'))
	require.NoError(t, err)
	_, err = f.Seek(0, 0)
	require.NoError(t, err)

	_, err = HashFD(f, 0)
	f.Close()
	assert.ErrorIs(t, err, ErrFileTooSmall)
}

// TestHashFDMaxSize verifies HashFD respects the max size limit.
func TestHashFDMaxSize(t *testing.T) {
	f, err := os.CreateTemp("", "ssdeep_big_*")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.Write(generateRandomData(64 * 1024))
	require.NoError(t, err)
	_, err = f.Seek(0, 0)
	require.NoError(t, err)

	_, err = HashFD(f, 8192)
	f.Close()
	assert.ErrorIs(t, err, ErrFileTooLarge)
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

// writeTempFile creates a temp file with the given data and returns the open file
// seeked back to the beginning.
func writeTempFile(b testing.TB, data []byte) *os.File {
	f, err := os.CreateTemp("", "ssdeep_bench_*")
	if err != nil {
		b.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		b.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		os.Remove(f.Name())
		b.Fatal(err)
	}
	return f
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

// ---------- In-memory benchmarks (pure hash throughput) ----------

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

func BenchmarkCGOWrite(b *testing.B) {
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

func BenchmarkCGOHashFD(b *testing.B) {
	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		f := writeTempFile(b, data)
		b.Cleanup(func() {
			f.Close()
			os.Remove(f.Name())
		})

		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			for b.Loop() {
				f.Seek(0, 0)
				HashFD(f, 0)
			}
		})
	}
}

// ---------- Real-world: read from disk + hash (apples-to-apples) ----------
// Simulates production where the file must be read from an fd.
// Glaslos uses Go io.CopyBuffer (32KB chunks), CGO HashFD uses C read().

func BenchmarkRealWorld_Glaslos(b *testing.B) {
	glaslos.Force = true
	defer func() { glaslos.Force = false }()

	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		f := writeTempFile(b, data)
		b.Cleanup(func() {
			f.Close()
			os.Remove(f.Name())
		})

		buf := make([]byte, 32*1024)
		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			for b.Loop() {
				f.Seek(0, 0)
				h := glaslos.New()
				for {
					n, err := f.Read(buf)
					if n > 0 {
						h.Write(buf[:n])
					}
					if err != nil {
						break
					}
				}
				h.Sum(nil)
			}
		})
	}
}

func BenchmarkRealWorld_CGOHashFD(b *testing.B) {
	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		f := writeTempFile(b, data)
		b.Cleanup(func() {
			f.Close()
			os.Remove(f.Name())
		})

		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			for b.Loop() {
				f.Seek(0, 0)
				HashFD(f, 0)
			}
		})
	}
}

// ---------- Concurrent benchmarks (production contention) ----------
// N goroutines each hashing their own file simultaneously.
// This exposes CGO thread pinning, scheduler thrashing, and GC pressure.

func BenchmarkConcurrent_Glaslos(b *testing.B) {
	glaslos.Force = true
	defer func() { glaslos.Force = false }()

	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				f := writeTempFile(b, data)
				defer func() {
					f.Close()
					os.Remove(f.Name())
				}()
				buf := make([]byte, 32*1024)
				for pb.Next() {
					f.Seek(0, 0)
					h := glaslos.New()
					for {
						n, err := f.Read(buf)
						if n > 0 {
							h.Write(buf[:n])
						}
						if err != nil {
							break
						}
					}
					h.Sum(nil)
				}
			})
		})
	}
}

func BenchmarkConcurrent_CGOWrite(b *testing.B) {
	Force = true
	defer func() { Force = false }()

	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				f := writeTempFile(b, data)
				defer func() {
					f.Close()
					os.Remove(f.Name())
				}()
				buf := make([]byte, 32*1024)
				for pb.Next() {
					f.Seek(0, 0)
					h := New()
					for {
						n, err := f.Read(buf)
						if n > 0 {
							h.Write(buf[:n])
						}
						if err != nil {
							break
						}
					}
					h.Sum(nil)
				}
			})
		})
	}
}

func BenchmarkConcurrent_CGOHashFD(b *testing.B) {
	for _, bs := range benchSizes {
		data := generateRandomData(bs.size)
		b.Run(bs.name, func(b *testing.B) {
			b.SetBytes(int64(bs.size))
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				f := writeTempFile(b, data)
				defer func() {
					f.Close()
					os.Remove(f.Name())
				}()
				for pb.Next() {
					f.Seek(0, 0)
					HashFD(f, 0)
				}
			})
		})
	}
}
