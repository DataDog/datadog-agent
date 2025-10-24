// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package seccomptracer

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSymbolicateAddressesWithDWARF(t *testing.T) {
	// Test that symbolication works with the new DWARF-based resolution
	// Use the current process as a test subject
	pid := uint32(os.Getpid())

	// Get some addresses from the current process
	// We'll use the address of a known function
	testAddrs := []uint64{
		0x400000, // Typical text segment start
		0x401000,
		0x402000,
	}

	symbols := SymbolicateAddresses(pid, testAddrs, SymbolicationModeRawAddresses|SymbolicationModeSymTable|SymbolicationModeDWARF)
	require.NotNil(t, symbols)
	require.Equal(t, len(testAddrs), len(symbols))

	// Verify all addresses were resolved to something
	for i, symbol := range symbols {
		assert.NotEmpty(t, symbol, "Address %d should be resolved", i)
		t.Logf("Address 0x%x -> %s", testAddrs[i], symbol)
	}
}

func TestSymbolicateAddressesCaching(t *testing.T) {
	pid := uint32(os.Getpid())
	testAddrs := []uint64{0x400000, 0x401000}

	// Clear cache to start fresh
	globalDwarfCache.Clear()
	assert.Equal(t, 0, globalDwarfCache.Len())

	// First symbolication - should populate cache
	symbols1 := SymbolicateAddresses(pid, testAddrs, SymbolicationModeRawAddresses|SymbolicationModeSymTable|SymbolicationModeDWARF)
	require.NotNil(t, symbols1)
	cacheSize := globalDwarfCache.Len()
	assert.Greater(t, cacheSize, 0, "Cache should be populated")

	// Second symbolication - should hit cache
	symbols2 := SymbolicateAddresses(pid, testAddrs, SymbolicationModeRawAddresses|SymbolicationModeSymTable|SymbolicationModeDWARF)
	require.NotNil(t, symbols2)
	assert.Equal(t, cacheSize, globalDwarfCache.Len(), "Cache size should not change on cache hit")

	// Symbols should be the same
	assert.Equal(t, symbols1, symbols2)
}

func TestSymbolicateAddressesFallback(t *testing.T) {
	// Test symbolication with invalid PID - should fall back to raw addresses
	invalidPID := uint32(999999)
	testAddrs := []uint64{0x12345678, 0x87654321}

	symbols := SymbolicateAddresses(invalidPID, testAddrs, SymbolicationModeRawAddresses|SymbolicationModeSymTable|SymbolicationModeDWARF)
	require.NotNil(t, symbols)
	require.Equal(t, len(testAddrs), len(symbols))

	// Should get raw addresses as fallback
	for i, symbol := range symbols {
		assert.Contains(t, symbol, "0x", "Should contain hex address")
		t.Logf("Fallback for address 0x%x -> %s", testAddrs[i], symbol)
	}
}

func TestSymbolicateAddressesEmpty(t *testing.T) {
	// Test with empty address list
	symbols := SymbolicateAddresses(1, nil, SymbolicationModeRawAddresses|SymbolicationModeSymTable|SymbolicationModeDWARF)
	assert.Nil(t, symbols)

	symbols = SymbolicateAddresses(1, []uint64{}, SymbolicationModeRawAddresses|SymbolicationModeSymTable|SymbolicationModeDWARF)
	assert.Nil(t, symbols)
}
