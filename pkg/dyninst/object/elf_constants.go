// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

// Normally the debug/elf package is not allowed to be used because
// it doesn't automatically recover from panics. Here we allow it just to
// get at some constants.
//
//nolint:depguard
import "debug/elf"

//nolint:revive
const (
	elf_COMPRESS_ZLIB = elf.COMPRESS_ZLIB
	elf_ELFCLASS32    = elf.ELFCLASS32
	elf_ELFCLASS64    = elf.ELFCLASS64

	elf_SHF_COMPRESSED = elf.SHF_COMPRESSED
	elf_SHT_NOBITS     = elf.SHT_NOBITS
	elf_SHF_ALLOC      = elf.SHF_ALLOC

	elf_ELFDATA2LSB = elf.ELFDATA2LSB
	elf_ELFDATA2MSB = elf.ELFDATA2MSB
)

//nolint:revive
type (
	elf_Chdr32          = elf.Chdr32
	elf_Chdr64          = elf.Chdr64
	elf_CompressionType = elf.CompressionType
)
