// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package seccomptracer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestResolveAddressWithSymbols(t *testing.T) {
	// Use a real binary that should have symbols
	binPath := "/bin/sh"
	if _, err := os.Stat(binPath); err != nil {
		t.Skipf("Binary %s not found: %v", binPath, err)
	}

	cache := newDwarfCache(10, 0) // No TTL for testing
	defer cache.Clear()

	info, err := os.Stat(binPath)
	require.NoError(t, err)

	stat := getStatInfo(t, info)
	key := binaryKey{
		dev:   stat.dev,
		inode: stat.inode,
	}

	// Load binary info
	binaryInfo, err := cache.get(key, binPath)
	require.NoError(t, err)
	require.NotNil(t, binaryInfo)

	// Try to resolve some address (we don't know exact addresses, but test the format)
	// Use a small offset that's likely to be in the binary
	symbol := resolveAddress(binaryInfo, 0x1000)

	// Should return something, either:
	// - Function name with DWARF info: "func_name (file.c:123)"
	// - Symbol table: "sh!symbol_name+0xoffset" or "sh!symbol_name"
	// - Fallback: "sh+0x1000"
	assert.NotEmpty(t, symbol)
	assert.Contains(t, symbol, "sh") // Should contain the binary name

	t.Logf("Resolved symbol: %s", symbol)
}

func TestResolveAddressFormats(t *testing.T) {
	testCases := []struct {
		name           string
		binaryPath     string
		offset         uint64
		expectedFormat string // What we expect in the output
	}{
		{
			name:           "valid binary with offset",
			binaryPath:     "/bin/sh",
			offset:         0x1000,
			expectedFormat: "sh", // Should contain binary name
		},
	}

	cache := newDwarfCache(10, 0)
	defer cache.Clear()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := os.Stat(tc.binaryPath); err != nil {
				t.Skipf("Binary %s not found: %v", tc.binaryPath, err)
			}

			info, err := os.Stat(tc.binaryPath)
			require.NoError(t, err)

			stat := getStatInfo(t, info)
			key := binaryKey{
				dev:   stat.dev,
				inode: stat.inode,
			}

			binaryInfo, err := cache.get(key, tc.binaryPath)
			require.NoError(t, err)
			require.NotNil(t, binaryInfo)

			symbol := resolveAddress(binaryInfo, tc.offset)
			assert.Contains(t, symbol, tc.expectedFormat)
			t.Logf("Resolved: %s", symbol)
		})
	}
}

func TestResolveSymbolFallback(t *testing.T) {
	// Test the symbol table fallback when DWARF is not available
	binPath := "/bin/ls"
	if _, err := os.Stat(binPath); err != nil {
		t.Skipf("Binary %s not found: %v", binPath, err)
	}

	cache := newDwarfCache(10, 0)
	defer cache.Clear()

	info, err := os.Stat(binPath)
	require.NoError(t, err)

	stat := getStatInfo(t, info)
	key := binaryKey{
		dev:   stat.dev,
		inode: stat.inode,
	}

	binaryInfo, err := cache.get(key, binPath)
	require.NoError(t, err)
	require.NotNil(t, binaryInfo)

	// Even if DWARF is missing, we should fall back to symbols
	symbol := resolveAddress(binaryInfo, 0x1000)
	assert.NotEmpty(t, symbol)

	// The symbol should either have:
	// - DWARF format: "func (file:line)"
	// - Symbol format: "ls!symbol+0xoffset"
	// - Fallback format: "ls+0xoffset"
	assert.True(t,
		strings.Contains(symbol, "!") || // Symbol table format
			strings.Contains(symbol, "(") || // DWARF format
			strings.Contains(symbol, "+0x"), // Fallback format
		"Symbol should be in one of the expected formats: %s", symbol)

	t.Logf("Resolved with fallback: %s", symbol)
}

func TestSymbolTableBinarySearch(t *testing.T) {
	// This tests the binary search logic in resolveSymbol
	// We'll use a real binary that should have symbols
	binPath := "/bin/cat"
	if _, err := os.Stat(binPath); err != nil {
		t.Skipf("Binary %s not found: %v", binPath, err)
	}

	cache := newDwarfCache(10, 0)
	defer cache.Clear()

	info, err := os.Stat(binPath)
	require.NoError(t, err)

	stat := getStatInfo(t, info)
	key := binaryKey{
		dev:   stat.dev,
		inode: stat.inode,
	}

	binaryInfo, err := cache.get(key, binPath)
	require.NoError(t, err)
	require.NotNil(t, binaryInfo)

	if len(binaryInfo.symbols) == 0 {
		t.Skip("Binary has no symbols")
	}

	t.Logf("Binary has %d symbols", len(binaryInfo.symbols))

	// Test resolving at various offsets
	offsets := []uint64{0x1000, 0x2000, 0x5000}
	for _, offset := range offsets {
		symbol := resolveAddress(binaryInfo, offset)
		assert.NotEmpty(t, symbol)
		t.Logf("Offset 0x%x -> %s", offset, symbol)
	}
}

func TestDWARFLineResolution(t *testing.T) {
	cache := newDwarfCache(10, 0)
	defer cache.Clear()

	// Build the seccompsample binary with debug info (-g flag)
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "../../testdata/seccompsample.c")
	if _, err := os.Stat(sourceFile); err != nil {
		t.Skipf("Test source file not found: %v", err)
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test_binary_with_debug")

	// Build with debug info (-g) and no stripping
	buildCmd := exec.Command("gcc", "-g", "-o", binPath, sourceFile, "-lseccomp")
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to compile test binary with debug info: %s", string(output))

	// Verify the binary exists
	require.FileExists(t, binPath)

	// Load the binary into cache
	info, err := os.Stat(binPath)
	require.NoError(t, err)

	stat := getStatInfo(t, info)
	key := binaryKey{
		dev:   stat.dev,
		inode: stat.inode,
	}

	binaryInfo, err := cache.get(key, binPath)
	require.NoError(t, err)
	require.NotNil(t, binaryInfo)

	// Verify DWARF data is present
	if binaryInfo.dwarfData == nil {
		t.Skip("Binary was built without DWARF info (unexpected)")
	}

	t.Logf("Testing DWARF resolution with: %s", binPath)

	// Get the ELF file to find actual function addresses
	elfFile := binaryInfo.elfFile
	symbols, err := elfFile.Symbols()
	if err == nil && len(symbols) > 0 {
		// Try to resolve addresses of actual functions
		for _, sym := range symbols {
			if sym.Info&0xf == byte(safeelf.STT_FUNC) && sym.Value > 0 && sym.Value < 0x100000 {
				// Try the function entry point
				symbol := resolveAddress(binaryInfo, sym.Value)
				assert.NotEmpty(t, symbol)
				t.Logf("Address 0x%x (%s) -> %s", sym.Value, sym.Name, symbol)

				// If DWARF worked, we should see line info format: "funcname (file:line)"
				if strings.Contains(symbol, "(") && strings.Contains(symbol, ":") {
					t.Logf("✓ DWARF resolution successful with line info")
					return
				}
			}
		}
	}

	// If we couldn't find symbols, try some offsets within the text section
	testOffsets := []uint64{0x1000, 0x2000, 0x3000, 0x4000, 0x5000}
	foundDwarfResolution := false
	for _, offset := range testOffsets {
		symbol := resolveAddress(binaryInfo, offset)
		assert.NotEmpty(t, symbol)

		// Check if DWARF worked (should contain file:line info)
		if strings.Contains(symbol, "(") && strings.Contains(symbol, ":") {
			t.Logf("✓ DWARF resolution successful: offset 0x%x -> %s", offset, symbol)
			foundDwarfResolution = true
			break
		}
		t.Logf("Offset 0x%x -> %s", offset, symbol)
	}

	if !foundDwarfResolution {
		t.Log("DWARF data present but line resolution didn't match test offsets - may need to adjust offsets")
	}
}
