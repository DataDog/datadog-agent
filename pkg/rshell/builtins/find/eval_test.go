// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find

import (
	"context"
	"io"
	iofs "io/fs"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/stretchr/testify/assert"
)

// TestEvalMminCeiling verifies that -mmin uses ceiling rounding.
// GNU find rounds up fractional minutes: a file 5 seconds old is in
// minute bucket 1 (not 0). This prevents regression to math.Floor.
func TestEvalMminCeiling(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		age     time.Duration // how old the file is
		n       int64
		cmp     cmpOp
		matched bool
	}{
		// Exact match uses ceiling bucketing: ceil(delta_sec / 60)
		// +N/-N use raw second comparison: delta_sec > N*60 / delta_sec < N*60

		// 0 seconds old → ceil(0) = 0 → bucket 0
		{"0s exact 0", 0, 0, cmpExact, true},
		{"0s gt 0", 0, 0, cmpMore, false}, // 0 > 0 = false
		{"0s lt 1", 0, 1, cmpLess, true},  // 0 < 60 = true

		// 1 second old → ceil(1/60) = 1 → bucket 1
		{"1s exact 0", 1 * time.Second, 0, cmpExact, false},
		{"1s exact 1", 1 * time.Second, 1, cmpExact, true},
		{"1s gt 0", 1 * time.Second, 0, cmpMore, true}, // 1 > 0 = true
		{"1s lt 1", 1 * time.Second, 1, cmpLess, true}, // 1 < 60 = true (GNU find matches)

		// 5 seconds old → ceil(5/60) = 1 → bucket 1
		{"5s exact 0", 5 * time.Second, 0, cmpExact, false},
		{"5s exact 1", 5 * time.Second, 1, cmpExact, true},
		{"5s gt 0", 5 * time.Second, 0, cmpMore, true}, // 5 > 0 = true
		{"5s lt 1", 5 * time.Second, 1, cmpLess, true}, // 5 < 60 = true (key regression test)

		// 30 seconds old — the specific case from codex P1
		{"30s lt 1", 30 * time.Second, 1, cmpLess, true}, // 30 < 60 = true

		// 59 seconds old → ceil(59/60) = 1 → bucket 1
		{"59s exact 1", 59 * time.Second, 1, cmpExact, true},
		{"59s exact 0", 59 * time.Second, 0, cmpExact, false},
		{"59s lt 1", 59 * time.Second, 1, cmpLess, true}, // 59 < 60 = true

		// 60 seconds old → ceil(60/60) = 1 → bucket 1
		{"60s exact 1", 60 * time.Second, 1, cmpExact, true},
		{"60s exact 2", 60 * time.Second, 2, cmpExact, false},
		{"60s gt 1", 60 * time.Second, 1, cmpMore, false}, // 60 > 60 = false
		{"60s lt 1", 60 * time.Second, 1, cmpLess, false}, // 60 < 60 = false

		// 61 seconds old → ceil(61/60) = 2 → bucket 2
		{"61s exact 1", 61 * time.Second, 1, cmpExact, false},
		{"61s exact 2", 61 * time.Second, 2, cmpExact, true},
		{"61s gt 1", 61 * time.Second, 1, cmpMore, true}, // 61 > 60 = true
		{"61s lt 2", 61 * time.Second, 2, cmpLess, true}, // 61 < 120 = true

		// 5 minutes old → ceil(300/60) = 5 → bucket 5
		{"5m exact 5", 5 * time.Minute, 5, cmpExact, true},
		{"5m gt 4", 5 * time.Minute, 4, cmpMore, true}, // 300 > 240 = true
		{"5m lt 6", 5 * time.Minute, 6, cmpLess, true}, // 300 < 360 = true

		// 5 minutes 1 second old → ceil(301/60) = 6 → bucket 6
		{"5m1s exact 6", 5*time.Minute + 1*time.Second, 6, cmpExact, true},
		{"5m1s exact 5", 5*time.Minute + 1*time.Second, 5, cmpExact, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modTime := now.Add(-tt.age)
			ec := &evalContext{
				now:  now,
				info: &fakeFileInfo{modTime: modTime},
			}
			got := evalMmin(ec, tt.n, tt.cmp)
			assert.Equal(t, tt.matched, got, "evalMmin(age=%v, n=%d, cmp=%s)", tt.age, tt.n, tt.cmp)
		})
	}
}

// TestEvalMminOverflow verifies that evalMmin handles values exceeding
// maxMminN without integer overflow. For +N (cmpMore), overflow values
// should return false (nothing qualifies). For -N (cmpLess), overflow
// values should return true (everything qualifies).
func TestEvalMminOverflow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// File is 1 hour old — a normal age for testing overflow thresholds.
	modTime := now.Add(-1 * time.Hour)
	ec := &evalContext{
		now:  now,
		info: &fakeFileInfo{modTime: modTime},
	}

	tests := []struct {
		name    string
		n       int64
		cmp     cmpOp
		matched bool
	}{
		// At the overflow boundary: maxMminN is the largest safe value.
		{"maxMminN +N", maxMminN, cmpMore, false},     // threshold is ~292K years; 1h file is newer
		{"maxMminN -N", maxMminN, cmpLess, true},      // 1h < ~292K years
		{"maxMminN exact", maxMminN, cmpExact, false}, // exact match impossible

		// Just past the boundary: these would overflow without the guard.
		{"maxMminN+1 +N", maxMminN + 1, cmpMore, false}, // overflow guard → false
		{"maxMminN+1 -N", maxMminN + 1, cmpLess, true},  // overflow guard → true

		// Very large values that would definitely overflow.
		{"huge +N", math.MaxInt64 / 2, cmpMore, false},
		{"huge -N", math.MaxInt64 / 2, cmpLess, true},
		{"maxint64 +N", math.MaxInt64, cmpMore, false},
		{"maxint64 -N", math.MaxInt64, cmpLess, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evalMmin(ec, tt.n, tt.cmp)
			assert.Equal(t, tt.matched, got, "evalMmin(n=%d, cmp=%s)", tt.n, tt.cmp)
		})
	}
}

// TestEvalMtimeFloor verifies that -mtime uses floor rounding (NOT ceiling).
// A file 5 hours old should be in day bucket 0 (not 1).
func TestEvalMtimeFloor(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		age     time.Duration
		n       int64
		cmp     cmpOp
		matched bool
	}{
		// 0 hours → floor(0/24) = 0
		{"0h exact 0", 0, 0, cmpExact, true},
		{"0h gt 0", 0, 0, cmpMore, false},

		// 5 hours → floor(5/24) = 0
		{"5h exact 0", 5 * time.Hour, 0, cmpExact, true},
		{"5h exact 1", 5 * time.Hour, 1, cmpExact, false},

		// 23 hours → floor(23/24) = 0
		{"23h exact 0", 23 * time.Hour, 0, cmpExact, true},

		// 24 hours → floor(24/24) = 1
		{"24h exact 1", 24 * time.Hour, 1, cmpExact, true},
		{"24h exact 0", 24 * time.Hour, 0, cmpExact, false},

		// 25 hours → floor(25/24) = 1
		{"25h exact 1", 25 * time.Hour, 1, cmpExact, true},

		// 48 hours → floor(48/24) = 2
		{"48h exact 2", 48 * time.Hour, 2, cmpExact, true},
		{"48h gt 1", 48 * time.Hour, 1, cmpMore, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modTime := now.Add(-tt.age)
			ec := &evalContext{
				now:  now,
				info: &fakeFileInfo{modTime: modTime},
			}
			got := evalMtime(ec, tt.n, tt.cmp)
			assert.Equal(t, tt.matched, got, "evalMtime(age=%v, n=%d, cmp=%s)", tt.age, tt.n, tt.cmp)
		})
	}
}

// TestCompareSizeOverflow verifies overflow-safe ceiling division.
func TestCompareSizeOverflow(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int64
		su       sizeUnit
		matched  bool
	}{
		// Normal cases
		{"0 bytes exact 0c", 0, sizeUnit{n: 0, cmp: cmpExact, unit: 'c'}, true},
		{"1 byte exact 1c", 1, sizeUnit{n: 1, cmp: cmpExact, unit: 'c'}, true},
		{"512 bytes exact 1b", 512, sizeUnit{n: 1, cmp: cmpExact, unit: 'b'}, true},
		{"1 byte rounds up to 1 block", 1, sizeUnit{n: 1, cmp: cmpExact, unit: 'b'}, true},
		{"513 bytes rounds up to 2 blocks", 513, sizeUnit{n: 2, cmp: cmpExact, unit: 'b'}, true},

		// Edge: zero-byte file
		{"0 bytes +0c", 0, sizeUnit{n: 0, cmp: cmpMore, unit: 'c'}, false},
		{"0 bytes -1c", 0, sizeUnit{n: 1, cmp: cmpLess, unit: 'c'}, true},

		// Large files near MaxInt64 (overflow protection)
		{"MaxInt64 bytes +0c", 1<<63 - 1, sizeUnit{n: 0, cmp: cmpMore, unit: 'c'}, true},
		{"MaxInt64 bytes exact in blocks", 1<<63 - 1, sizeUnit{n: (1<<63 - 1) / 512, cmp: cmpMore, unit: 'b'}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareSize(tt.fileSize, tt.su)
			assert.Equal(t, tt.matched, got)
		})
	}
}

// TestEvalEmpty verifies the -empty predicate for directories, regular files,
// and other file types. Scenario tests cannot create empty dirs (setup.files
// requires a file), so directory emptiness must be tested here.
func TestEvalEmpty(t *testing.T) {
	t.Run("empty directory matches", func(t *testing.T) {
		called := false
		ec := &evalContext{
			ctx:       context.Background(),
			info:      &fakeFileInfo{isDir: true},
			printPath: "emptydir",
			callCtx: &builtins.CallContext{
				Stderr: io.Discard,
				IsDirEmpty: func(_ context.Context, _ string) (bool, error) {
					called = true
					return true, nil
				},
			},
		}
		assert.True(t, evalEmpty(ec), "empty directory should match -empty")
		assert.True(t, called, "IsDirEmpty must be called for directories")
	})

	t.Run("non-empty directory does not match", func(t *testing.T) {
		ec := &evalContext{
			ctx:       context.Background(),
			info:      &fakeFileInfo{isDir: true},
			printPath: "nonemptydir",
			callCtx: &builtins.CallContext{
				Stderr: io.Discard,
				IsDirEmpty: func(_ context.Context, _ string) (bool, error) {
					return false, nil
				},
			},
		}
		assert.False(t, evalEmpty(ec), "non-empty directory should not match -empty")
	})

	t.Run("IsDirEmpty receives correct path", func(t *testing.T) {
		var gotPath string
		ec := &evalContext{
			ctx:       context.Background(),
			info:      &fakeFileInfo{isDir: true},
			printPath: "some/nested/dir",
			callCtx: &builtins.CallContext{
				Stderr: io.Discard,
				IsDirEmpty: func(_ context.Context, path string) (bool, error) {
					gotPath = path
					return true, nil
				},
			},
		}
		evalEmpty(ec)
		assert.Equal(t, "some/nested/dir", gotPath, "IsDirEmpty should receive printPath")
	})

	t.Run("IsDirEmpty error sets failed and returns false", func(t *testing.T) {
		var stderr strings.Builder
		ec := &evalContext{
			ctx:       context.Background(),
			info:      &fakeFileInfo{isDir: true},
			printPath: "baddir",
			callCtx: &builtins.CallContext{
				Stderr: &stderr,
				IsDirEmpty: func(_ context.Context, _ string) (bool, error) {
					return false, &iofs.PathError{Op: "readdir", Path: "baddir", Err: iofs.ErrPermission}
				},
				PortableErr: func(err error) string { return err.Error() },
			},
		}
		assert.False(t, evalEmpty(ec), "error should return false")
		assert.True(t, ec.failed, "error should set failed flag")
		assert.Contains(t, stderr.String(), "baddir", "error should mention the path on stderr")
	})

	t.Run("empty regular file matches", func(t *testing.T) {
		ec := &evalContext{
			ctx:  context.Background(),
			info: &fakeFileInfo{size: 0, isDir: false},
		}
		assert.True(t, evalEmpty(ec), "zero-byte regular file should match -empty")
	})

	t.Run("non-empty regular file does not match", func(t *testing.T) {
		ec := &evalContext{
			ctx:  context.Background(),
			info: &fakeFileInfo{size: 42, isDir: false},
		}
		assert.False(t, evalEmpty(ec), "non-empty regular file should not match -empty")
	})

	t.Run("symlink does not match", func(t *testing.T) {
		ec := &evalContext{
			ctx:  context.Background(),
			info: &fakeFileInfo{mode: iofs.ModeSymlink},
		}
		assert.False(t, evalEmpty(ec), "symlink should not match -empty")
	})

	t.Run("IsDirEmpty not called for regular files", func(t *testing.T) {
		called := false
		ec := &evalContext{
			ctx:  context.Background(),
			info: &fakeFileInfo{size: 0, isDir: false},
			callCtx: &builtins.CallContext{
				IsDirEmpty: func(_ context.Context, _ string) (bool, error) {
					called = true
					return true, nil
				},
			},
		}
		evalEmpty(ec)
		assert.False(t, called, "IsDirEmpty should not be called for regular files")
	})
}

// fakeDirEntry implements a minimal fs.DirEntry for testing.
type fakeDirEntry struct{}

func (fakeDirEntry) Name() string                 { return "file.txt" }
func (fakeDirEntry) IsDir() bool                  { return false }
func (fakeDirEntry) Type() iofs.FileMode          { return 0 }
func (fakeDirEntry) Info() (iofs.FileInfo, error) { return nil, nil }

// fakeFileInfo implements the minimal fs.FileInfo interface for testing.
type fakeFileInfo struct {
	modTime time.Time
	size    int64
	isDir   bool
	mode    iofs.FileMode // when set, Mode() returns this directly
}

func (f *fakeFileInfo) Name() string       { return "fake" }
func (f *fakeFileInfo) Size() int64        { return f.size }
func (f *fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f *fakeFileInfo) IsDir() bool        { return f.isDir }
func (f *fakeFileInfo) Sys() any           { return nil }

// Mode returns a basic file mode for testing. If mode is explicitly set,
// it is returned directly; otherwise a default is derived from isDir.
func (f *fakeFileInfo) Mode() iofs.FileMode {
	if f.mode != 0 {
		return f.mode
	}
	if f.isDir {
		return iofs.ModeDir | 0o755
	}
	return 0o644
}
