// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package utils

import (
	"bytes"
	"debug/elf"
	"encoding/hex"
	"fmt"
)

var (
	magicInfoStart, _ = hex.DecodeString("3077af0c9274080241e1c107e6d618e6")
	magicInfoEnd, _   = hex.DecodeString("f932433186182072008242104116d8f2")
)

// GetElfModuleBuildInfos parses the given path expecting to lookup an ELF
// file compiled with Go. It then extracts the info section that contains
// build and dep informations to extract only the build metadata.
//
// This code is partially ported from https://github.com/rsc/goversion but we
// extracted only the logic relative to ELF files.
//
// Copyright (c) 2009 The Go Authors. All rights reserved.
func GetElfModuleBuildInfos(name string) ([]string, error) {
	exe, err := elf.Open(name)
	if err != nil {
		return nil, err
	}
	defer exe.Close()

	var ro *elf.Prog
	for _, p := range exe.Progs {
		if p.Type == elf.PT_LOAD && p.Flags&(elf.PF_R|elf.PF_W|elf.PF_X) == elf.PF_R {
			ro = p
			break
		}
	}
	if ro == nil {
		for _, p := range exe.Progs {
			if p.Type == elf.PT_LOAD && p.Flags&(elf.PF_R|elf.PF_W|elf.PF_X) == (elf.PF_R|elf.PF_X) {
				ro = p
				break
			}
		}
	}
	if ro == nil {
		return nil, fmt.Errorf("could not locate RO data section")
	}

	const maxModInfo = 128 << 10
	const maxSize = uint64(4 << 20)

	start, end := ro.Vaddr, ro.Vaddr+ro.Filesz
	buf := make([]byte, maxSize)
	for addr := start; addr < end; {
		size := maxSize
		if end-addr < maxSize {
			size = end - addr
		}
		var data []byte
		for _, prog := range exe.Progs {
			if prog.Vaddr <= addr && addr <= prog.Vaddr+prog.Filesz-1 {
				n := prog.Vaddr + prog.Filesz - addr
				if n > size {
					n = size
				}
				data = buf[:n]
				_, err := prog.ReadAt(data, int64(addr-prog.Vaddr))
				if err != nil {
					return nil, err
				}
				break
			}
		}
		if i := bytes.Index(data, magicInfoStart); i >= 0 {
			if j := bytes.Index(data[i:], magicInfoEnd); j >= 0 {
				moduleInfoSection := data[i+len(magicInfoStart) : i+j]
				moduleInfos := bytes.Split(moduleInfoSection, []byte("\n"))
				var buildInfos []string
				const buildPrefix = "build\t"
				for _, info := range moduleInfos {
					if bytes.HasPrefix(info, []byte(buildPrefix)) {
						buildInfo := bytes.TrimPrefix(info, []byte(buildPrefix))
						buildInfos = append(buildInfos, string(buildInfo))
					}
				}
				return buildInfos, nil
			}
		}
		if addr+size < end {
			size -= maxModInfo
		}
		addr += size
	}
	return nil, fmt.Errorf("could not local module infos in ELF")
}
