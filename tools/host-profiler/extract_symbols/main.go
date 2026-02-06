// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main provides a tool to extract debug symbols from ELF files.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbolcopier"
)

func extractDebugInfos(elfFile, outFile string) error {
	ef, err := symbol.NewElfFromDisk(elfFile)
	if err != nil {
		return fmt.Errorf("failed to open elf file: %w", err)
	}
	defer ef.Close()

	goPCLnTabInfo, err := ef.GoPCLnTab()
	if ef.IsGolang() {
		if err != nil {
			return fmt.Errorf("failed to find pclntab: %w", err)
		}

		fmt.Printf("Found GoPCLnTab at 0x%x, size %d, headerVersion: %v\n", goPCLnTabInfo.Address, len(goPCLnTabInfo.Data), goPCLnTabInfo.Version.String())
		fmt.Printf("Found GoFunc at 0x%x, size %d\n", goPCLnTabInfo.GoFuncAddr, len(goPCLnTabInfo.GoFuncData))
	}

	var sectionsToKeep []symbol.SectionInfo
	if ef.SymbolSource() == symbol.SourceDynamicSymbolTable {
		sectionsToKeep = ef.GetSectionsRequiredForDynamicSymbols()
	}

	return symbolcopier.CopySymbols(context.Background(), elfFile, outFile, goPCLnTabInfo, sectionsToKeep, symboluploader.CheckObjcopyZstdSupport(context.Background()))
}

func main() {
	if len(os.Args) != 3 {
		fmt.Printf("Usage: %s <elf-file> <debug-file>\n", os.Args[0])
		return
	}

	elfFile := os.Args[1]
	outFile := os.Args[2]

	err := extractDebugInfos(elfFile, outFile)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
