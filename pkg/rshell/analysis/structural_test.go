// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ScannerBuffer rule tests ---

func TestScannerBufferClean(t *testing.T) {
	// Scanner with Buffer() called — must not be flagged.
	fset := token.NewFileSet()
	src := `package p
import "bufio"
func run() {
	sc := bufio.NewScanner(nil)
	sc.Buffer(make([]byte, 4096), 1<<20)
	_ = sc.Scan()
}
`
	f, err := parser.ParseFile(fset, "clean.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileScannerBuffer(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Empty(t, violations, "Scanner with Buffer() should not be flagged")
}

func TestScannerBufferMissing(t *testing.T) {
	// Scanner without Buffer() — must be flagged.
	fset := token.NewFileSet()
	src := `package p
import "bufio"
func run() {
	sc := bufio.NewScanner(nil)
	_ = sc.Scan()
}
`
	f, err := parser.ParseFile(fset, "missing.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileScannerBuffer(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Len(t, violations, 1, "Scanner without Buffer() must be flagged")
}

func TestScannerBufferInsideFuncLit(t *testing.T) {
	// Scanner inside a func literal without Buffer() — must be flagged as its own scope.
	fset := token.NewFileSet()
	src := `package p
import "bufio"
func run() {
	fn := func() {
		sc := bufio.NewScanner(nil)
		_ = sc.Scan()
	}
	fn()
}
`
	f, err := parser.ParseFile(fset, "funlit.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileScannerBuffer(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Len(t, violations, 1, "Scanner inside func literal without Buffer() must be flagged")
}

func TestScannerBufferOuterCleanInnerMissing(t *testing.T) {
	// Outer function has Buffer(), inner func literal does not — only inner flagged.
	fset := token.NewFileSet()
	src := `package p
import "bufio"
func run() {
	sc := bufio.NewScanner(nil)
	sc.Buffer(make([]byte, 4096), 1<<20)
	_ = sc.Scan()

	inner := func() {
		sc2 := bufio.NewScanner(nil)
		_ = sc2.Scan()
	}
	inner()
}
`
	f, err := parser.ParseFile(fset, "mixed.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileScannerBuffer(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Len(t, violations, 1, "only the inner scanner without Buffer() must be flagged")
}

// --- OpenFileClose rule tests ---

func TestOpenFileCloseClean(t *testing.T) {
	// File opened and closed via defer — must not be flagged.
	fset := token.NewFileSet()
	src := `package p
import ("context"; "os")
type cc struct{}
func (cc) OpenFile(ctx context.Context, path string, flags int, mode os.FileMode) (interface{ Read([]byte)(int,error); Close() error }, error) { return nil, nil }
func run(callCtx cc) {
	f, err := callCtx.OpenFile(context.Background(), "x", os.O_RDONLY, 0)
	if err != nil { return }
	defer f.Close()
}
`
	f, err := parser.ParseFile(fset, "clean.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileOpenFileClose(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Empty(t, violations, "OpenFile with defer f.Close() should not be flagged")
}

func TestOpenFileCloseHandOff(t *testing.T) {
	// File opened, handed off to rc, then rc closed — must not be flagged.
	fset := token.NewFileSet()
	src := `package p
import ("context"; "io"; "os")
type cc struct{}
func (cc) OpenFile(ctx context.Context, path string, flags int, mode os.FileMode) (interface{ Read([]byte)(int,error); Close() error }, error) { return nil, nil }
func run(callCtx cc) {
	var rc io.ReadCloser
	f, err := callCtx.OpenFile(context.Background(), "x", os.O_RDONLY, 0)
	if err != nil { return }
	rc = f
	defer rc.Close()
}
`
	f, err := parser.ParseFile(fset, "handoff.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileOpenFileClose(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Empty(t, violations, "OpenFile handed off to rc and rc closed should not be flagged")
}

func TestOpenFileCloseMissing(t *testing.T) {
	// File opened but never closed — must be flagged.
	fset := token.NewFileSet()
	src := `package p
import ("context"; "os")
type cc struct{}
func (cc) OpenFile(ctx context.Context, path string, flags int, mode os.FileMode) (interface{ Read([]byte)(int,error); Close() error }, error) { return nil, nil }
func run(callCtx cc) {
	f, err := callCtx.OpenFile(context.Background(), "x", os.O_RDONLY, 0)
	if err != nil { return }
	_ = f
}
`
	f, err := parser.ParseFile(fset, "missing.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileOpenFileClose(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Len(t, violations, 1, "OpenFile result not closed must be flagged")
}

func TestOpenFileCloseReturnTransfer(t *testing.T) {
	// File opened then returned to caller — must not be flagged.
	fset := token.NewFileSet()
	src := `package p
import ("context"; "os"; "io")
type cc struct{}
func (cc) OpenFile(ctx context.Context, path string, flags int, mode os.FileMode) (interface{ Read([]byte)(int,error); Close() error }, error) { return nil, nil }
func open(callCtx cc) (io.ReadCloser, error) {
	f, err := callCtx.OpenFile(context.Background(), "x", os.O_RDONLY, 0)
	if err != nil { return nil, err }
	return f, nil
}
`
	f, err := parser.ParseFile(fset, "return.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileOpenFileClose(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Empty(t, violations, "OpenFile result returned to caller should not be flagged")
}

func TestOpenFileCloseChainedHandOff(t *testing.T) {
	// f → a → b → Close(): two-hop chain must be detected.
	fset := token.NewFileSet()
	src := `package p
import ("context"; "io"; "os")
type cc struct{}
func (cc) OpenFile(ctx context.Context, path string, flags int, mode os.FileMode) (interface{ Read([]byte)(int,error); Close() error }, error) { return nil, nil }
func run(callCtx cc) {
	f, err := callCtx.OpenFile(context.Background(), "x", os.O_RDONLY, 0)
	if err != nil { return }
	var a io.ReadCloser
	a = f
	var b io.ReadCloser
	b = a
	defer b.Close()
}
`
	f, err := parser.ParseFile(fset, "chain.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileOpenFileClose(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	assert.Empty(t, violations, "two-hop hand-off chain closed at end must not be flagged")
}

// TestOpenFileCloseKnownLimitationMultiBranchReturn documents a known
// path-insensitivity limitation: if a variable appears in a return statement
// on any branch, the checker treats the whole variable as "returned" and will
// not flag it even if another branch leaks it. Fixing this requires CFG-based
// data-flow analysis beyond the scope of this AST-only checker.
func TestOpenFileCloseKnownLimitationMultiBranchReturn(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
import ("context"; "io"; "os")
type cc struct{}
func (cc) OpenFile(ctx context.Context, path string, flags int, mode os.FileMode) (interface{ Read([]byte)(int,error); Close() error }, error) { return nil, nil }
func run(callCtx cc, cond bool) (io.ReadCloser, error) {
	f, err := callCtx.OpenFile(context.Background(), "x", os.O_RDONLY, 0)
	if err != nil { return nil, err }
	if cond {
		return f, nil // f is returned here
	}
	// f is leaked here (no close, no return) — NOT caught due to path-insensitivity
	return nil, nil
}
`
	f, err := parser.ParseFile(fset, "multibranch.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileOpenFileClose(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	// Known false negative: the checker sees "f" in a return statement and
	// exempts it globally. This test documents the gap rather than asserting
	// correct behaviour.
	assert.Empty(t, violations, "known limitation: path-insensitive return exemption produces false negative")
}

// TestOpenFileCloseKnownLimitationNameReuse documents a known limitation:
// if the same variable name is reused for two successive OpenFile results,
// a single Close() satisfies both entries in the checker.
func TestOpenFileCloseKnownLimitationNameReuse(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
import ("context"; "os")
type cc struct{}
func (cc) OpenFile(ctx context.Context, path string, flags int, mode os.FileMode) (interface{ Read([]byte)(int,error); Close() error }, error) { return nil, nil }
func run(callCtx cc) {
	f, err := callCtx.OpenFile(context.Background(), "x", os.O_RDONLY, 0)
	if err != nil { return }
	defer f.Close()
	// Re-using f for a second OpenFile — the second handle is never closed.
	f, err = callCtx.OpenFile(context.Background(), "y", os.O_RDONLY, 0)
	if err != nil { return }
	_ = f
}
`
	f, err := parser.ParseFile(fset, "reuse.go", src, 0)
	require.NoError(t, err)

	var violations []string
	checkFileOpenFileClose(f, func(_ token.Pos, format string, _ ...any) {
		violations = append(violations, format)
	})
	// Known false negative: closed["f"]=true from the first defer satisfies
	// both opens entries. Use distinct variable names in production code.
	assert.Empty(t, violations, "known limitation: name reuse produces false negative")
}
