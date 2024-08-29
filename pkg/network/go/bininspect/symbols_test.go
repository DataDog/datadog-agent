// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"debug/elf"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

func openTestElf(t *testing.T) *elf.File {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	// It doesn't matter which architecture we are on, so just open arm64
	// always.
	lib := filepath.Join(curDir, "..", "..", "usm", "testdata",
		"site-packages", "ddtrace", "libssl.so.arm64")
	elfFile, err := elf.Open(lib)
	require.NoError(t, err)

	return elfFile
}

func TestAllFound(t *testing.T) {
	elfFile := openTestElf(t)
	// The test ELF currently only has dynamic symbols.
	allSymbols, err := elfFile.DynamicSymbols()
	require.NoError(t, err)
	require.NotEmpty(t, allSymbols)

	symbolSet := make(common.StringSet, len(allSymbols))
	for _, sym := range allSymbols {
		symbolSet[sym.Name] = struct{}{}
	}

	symbols, err := GetAllSymbolsInSetByName(elfFile, symbolSet)
	require.NoError(t, err)
	for sym := range symbolSet {
		require.Contains(t, symbols, sym)
		require.Equal(t, symbols[sym].Name, sym)
	}
}

func TestAllMissing(t *testing.T) {
	elfFile := openTestElf(t)
	symbolSet := common.StringSet{
		"SSL_connect_not": {},
		"foo":             {},
	}

	_, err := GetAllSymbolsInSetByName(elfFile, symbolSet)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "SSL_connect_not")
	assert.Contains(t, msg, "foo")
}

func TestSomeMissing(t *testing.T) {
	elfFile := openTestElf(t)
	symbolSet := common.StringSet{
		"SSL_connect":  {},
		"SSL_invalid":  {},
		"SSL_set_bio":  {},
		"SSL_notthere": {},
	}

	_, err := GetAllSymbolsInSetByName(elfFile, symbolSet)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "SSL_invalid")
	assert.Contains(t, msg, "SSL_notthere")
	assert.NotContains(t, msg, "SSL_connect")
	assert.NotContains(t, msg, "SSL_set_bio")
}
