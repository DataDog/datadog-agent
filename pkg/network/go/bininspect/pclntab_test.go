// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"debug/elf"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"strconv"
	"strings"
	"testing"
)

const (
	// Info is composed of the type and binding of the symbol. Type is the lower 4 bits and binding is the upper 4 bits.
	// We are only interested in functions, which binding STB_GLOBAL (1) and type STT_FUNC (2).
	// Hence, we are interested in symbols with Info 18.
	infoFunction = byte(elf.STB_GLOBAL)<<4 | byte(elf.STT_FUNC)
)

// TestGetPCLNTABSymbolParser tests the GetPCLNTABSymbolParser function with strings set symbol filter.
// We are looking to find all symbols of the current process executable and check if they are found in the PCLNTAB.
func TestGetPCLNTABSymbolParser(t *testing.T) {
	currentPid := os.Getpid()
	f, err := elf.Open("/proc/" + strconv.Itoa(currentPid) + "/exe")
	require.NoError(t, err)
	symbolSet := make(common.StringSet)
	staticSymbols, _ := f.Symbols()
	dynamicSymbols, _ := f.DynamicSymbols()
	for _, symbols := range [][]elf.Symbol{staticSymbols, dynamicSymbols} {
		for _, sym := range symbols {
			if sym.Info != infoFunction {
				continue
			}
			// Skipping types, runtime functions and ABI0 functions
			if strings.HasPrefix(sym.Name, "type:") || strings.HasPrefix(sym.Name, "runtime") || strings.HasSuffix(sym.Name, ".abi0") {
				continue
			}
			symbolSet[sym.Name] = struct{}{}
		}
	}
	if len(symbolSet) == 0 {
		t.Skip("No symbols found")
	}

	got, err := GetPCLNTABSymbolParser(f, newStringSetSymbolFilter(symbolSet))
	assert.NoError(t, err)
	if err != nil {
		for sym := range symbolSet {
			if _, ok := got[sym]; !ok {
				t.Log("Missing symbol:", sym)
			}
		}
	}
}
