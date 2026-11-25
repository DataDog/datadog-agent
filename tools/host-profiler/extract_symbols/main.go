// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main provides a tool to extract debug symbols from ELF files.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/pclntab"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbolcopier"
)

func extractDebugInfos(elfFile, outFile string, stream bool) error {
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

	if stream {
		return streamSymbols(elfFile, outFile, goPCLnTabInfo, sectionsToKeep)
	}
	return symbolcopier.CopySymbols(context.Background(), elfFile, outFile, goPCLnTabInfo, sectionsToKeep)
}

// streamSymbols exports elfFile with elfstream, as the uploader does with
// stream_symbols, but uncompressed so ELF tools can read the result.
func streamSymbols(elfFile, outFile string, goPCLnTabInfo *pclntab.GoPCLnTabInfo, sectionsToKeep []symbol.SectionInfo) error {
	input, err := os.Open(elfFile)
	if err != nil {
		return err
	}
	defer input.Close()
	stat, err := input.Stat()
	if err != nil {
		return err
	}
	ctx := context.Background()
	stream, err := symbolcopier.NewSymbolStream(ctx, input, stat.Size(), goPCLnTabInfo, sectionsToKeep)
	if err != nil {
		return err
	}
	output, err := os.Create(outFile)
	if err != nil {
		return err
	}
	return errors.Join(stream.Write(ctx, output), output.Close())
}

func main() {
	stream := flag.Bool("stream", false, "export with elfstream instead of objcopy")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Printf("Usage: %s [-stream] <elf-file> <debug-file>\n", os.Args[0])
		return
	}

	err := extractDebugInfos(flag.Arg(0), flag.Arg(1), *stream)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
