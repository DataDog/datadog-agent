// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package symbolcopier provides utilities for copying debug symbols from ELF files.
package symbolcopier

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/oom"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/pclntab"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
	elf "github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

type goPCLnTabDump struct {
	goPCLnTabPath string
	goFuncPath    string
}

func (g *goPCLnTabDump) Remove() {
	os.Remove(g.goPCLnTabPath)
	if g.goFuncPath != "" {
		os.Remove(g.goFuncPath)
	}
}

func dumpGoPCLnTabData(goPCLnTabInfo *pclntab.GoPCLnTabInfo) (goPCLnTabDump, error) {
	// Dump gopclntab data to a temporary file
	gopclntabFile, err := os.CreateTemp("", "gopclntab")
	if err != nil {
		return goPCLnTabDump{}, fmt.Errorf("failed to create temp file to extract GoPCLnTab: %w", err)
	}
	defer func() {
		gopclntabFile.Close()
		if err != nil {
			os.Remove(gopclntabFile.Name())
		}
	}()

	_, err = gopclntabFile.Write(goPCLnTabInfo.Data)
	if err != nil {
		return goPCLnTabDump{}, fmt.Errorf("failed to write GoPCLnTab: %w", err)
	}

	if goPCLnTabInfo.GoFuncData == nil {
		return goPCLnTabDump{goPCLnTabPath: gopclntabFile.Name()}, nil
	}

	// Dump gofunc data to a temporary file
	gofuncFile, err := os.CreateTemp("", "gofunc")
	if err != nil {
		return goPCLnTabDump{}, fmt.Errorf("failed to create temp file to extract GoFunc: %w", err)
	}
	defer func() {
		gofuncFile.Close()
		if err != nil {
			os.Remove(gofuncFile.Name())
		}
	}()

	_, err = gofuncFile.Write(goPCLnTabInfo.GoFuncData)
	if err != nil {
		return goPCLnTabDump{}, fmt.Errorf("failed to write GoFunc: %w", err)
	}

	return goPCLnTabDump{goPCLnTabPath: gopclntabFile.Name(), goFuncPath: gofuncFile.Name()}, nil
}

func CopySymbols(ctx context.Context, inputPath, outputPath string, goPCLnTabInfo *pclntab.GoPCLnTabInfo,
	sectionsToKeep []symbol.SectionInfo, compressDebugSections bool) error {
	args := []string{
		"--only-keep-debug",
		"--remove-section=.gdb_index",
	}

	if goPCLnTabInfo != nil {
		// objcopy does not support extracting debug information (with `--only-keep-debug`) and keeping
		// some non-debug sections (like gopclntab) at the same time.
		// `--only-keep-debug` does not really remove non-debug sections, it keeps their memory size
		// but makes their file size 0 by marking them NOBITS (effectively zeroing them).
		// That's why we extract debug information and at the same time remove `.gopclntab` section (with
		// with `--remove-section=.gopclntab`) and add it back from the temporary file.
		gopclntabDump, err := dumpGoPCLnTabData(goPCLnTabInfo)
		if err != nil {
			return fmt.Errorf("failed to dump GoPCLnTab data: %w", err)
		}
		defer gopclntabDump.Remove()

		args = append(args,
			"--remove-section=.gopclntab",
			"--remove-section=.data.rel.ro.gopclntab",
			"--add-section", ".gopclntab="+gopclntabDump.goPCLnTabPath,
			"--set-section-flags", ".gopclntab=readonly",
			fmt.Sprintf("--change-section-address=.gopclntab=%d", goPCLnTabInfo.Address))

		if gopclntabDump.goFuncPath != "" {
			args = append(args, "--add-section", ".gofunc="+gopclntabDump.goFuncPath,
				"--set-section-flags", ".gofunc=readonly",
				fmt.Sprintf("--change-section-address=.gofunc=%d", goPCLnTabInfo.GoFuncAddr),
				"--strip-symbol", "go:func.*",
				"--add-symbol", "go:func.*=.gofunc:0")
		} else if goPCLnTabInfo.GoFuncAddr != 0 {
			// Gofunc is in .gopclntab, no need to add a new section for it.
			// Just add a symbol so that we can find it easily later.
			args = append(args, "--strip-symbol", "go:func.*",
				"--add-symbol", fmt.Sprintf("go:func.*=.gopclntab:%d", goPCLnTabInfo.GoFuncAddr-goPCLnTabInfo.Address))
		}

		if goPCLnTabInfo.TextStart.Address != 0 && goPCLnTabInfo.TextStart.Origin == pclntab.TextStartOriginModuleData {
			// If the text start can only be found in moduledata, we need to add a symbol so that we can find it easily later.
			args = append(args, "--strip-symbol", "runtime.text", "--add-symbol",
				fmt.Sprintf("runtime.text=.text:%d", goPCLnTabInfo.TextStart.Address-goPCLnTabInfo.TextStart.TextSectionAddress))
		}
	}

	for _, section := range sectionsToKeep {
		args = append(args, "--set-section-flags", fmt.Sprintf("%s=%s", section.Name, getStringFlags(section.Flags)))
	}

	if compressDebugSections {
		args = append(args, "--compress-debug-sections=zstd")
	}

	args = append(args, inputPath, outputPath)

	cmd := exec.CommandContext(ctx, "objcopy", args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start copying symbols: %w", err)
	}

	// Increase probability that objcopy gets killed in case the cgroup is OOM
	if err := oom.SetOOMScoreAdj(cmd.Process.Pid, 1000); err != nil {
		slog.Warn("Could not adjust OOM score", slog.String("error", err.Error()))
	}

	return cmd.Wait()
}

func getStringFlags(flags elf.SectionFlag) string {
	flagsStr := "contents"
	if flags&elf.SHF_WRITE == 0 {
		flagsStr += ",readonly"
	}
	if flags&elf.SHF_ALLOC != 0 {
		flagsStr += ",alloc"
	}
	if flags&elf.SHF_EXECINSTR != 0 {
		flagsStr += ",exec"
	}
	return flagsStr
}
