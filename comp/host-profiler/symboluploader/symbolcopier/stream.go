// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package symbolcopier

import (
	"context"
	"io"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/pclntab"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
	"github.com/DataDog/datadog-agent/pkg/util/elfstream"
)

// keepGoSections adds the Go tables and markers that objcopy's path adds.
func keepGoSections(goInfo *pclntab.GoPCLnTabInfo, opts *elfstream.StreamOptions) {
	opts.Sections = append(opts.Sections, elfstream.StreamSection{Name: ".gopclntab", Address: goInfo.Address, Data: goInfo.Data})
	if len(goInfo.GoFuncData) != 0 {
		opts.Sections = append(opts.Sections, elfstream.StreamSection{Name: ".gofunc", Address: goInfo.GoFuncAddr, Data: goInfo.GoFuncData})
		opts.Symbols = append(opts.Symbols, elfstream.StreamSymbol{Name: "go:func.*", Section: ".gofunc", Value: goInfo.GoFuncAddr})
	} else if goInfo.GoFuncAddr != 0 {
		// Some Go versions embed function metadata inside pclntab.
		opts.Symbols = append(opts.Symbols, elfstream.StreamSymbol{Name: "go:func.*", Section: ".gopclntab", Value: goInfo.GoFuncAddr})
	}
	if goInfo.TextStart.Address != 0 && goInfo.TextStart.Origin == pclntab.TextStartOriginModuleData {
		// Restore the text-base marker recovered from runtime metadata.
		opts.Symbols = append(opts.Symbols, elfstream.StreamSymbol{Name: "runtime.text", Section: ".text", Value: goInfo.TextStart.Address})
	}
}

// NewSymbolStream plans a symbol ELF from input, keeping the given sections and
// the Go tables in goInfo. Keep input and goInfo's data unchanged until the
// stream is written.
func NewSymbolStream(ctx context.Context, input io.ReaderAt, size int64, goInfo *pclntab.GoPCLnTabInfo, keep []symbol.SectionInfo) (*elfstream.ELFStream, error) {
	var opts elfstream.StreamOptions
	for _, section := range keep {
		opts.KeepSections = append(opts.KeepSections, section.Name)
	}
	if goInfo != nil {
		keepGoSections(goInfo, &opts)
	}
	return elfstream.NewELFStream(ctx, input, size, opts)
}
