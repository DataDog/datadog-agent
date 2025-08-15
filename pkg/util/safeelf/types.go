// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//nolint:revive
package safeelf

import "debug/elf" //nolint:depguard

type Prog = elf.Prog
type Symbol = elf.Symbol
type Section = elf.Section
type SectionHeader = elf.SectionHeader
type SectionType = elf.SectionType
type SectionIndex = elf.SectionIndex

var ErrNoSymbols = elf.ErrNoSymbols

const SHF_ALLOC = elf.SHF_ALLOC
const SHF_EXECINSTR = elf.SHF_EXECINSTR

const SHT_SYMTAB = elf.SHT_SYMTAB
const SHT_DYNSYM = elf.SHT_DYNSYM
const SHT_NOTE = elf.SHT_NOTE

const ET_EXEC = elf.ET_EXEC
const ET_DYN = elf.ET_DYN

const PT_LOAD = elf.PT_LOAD
const PT_TLS = elf.PT_TLS

const EM_X86_64 = elf.EM_X86_64
const EM_AARCH64 = elf.EM_AARCH64

const Sym32Size = elf.Sym32Size
const Sym64Size = elf.Sym64Size

const ELFCLASS32 = elf.ELFCLASS32
const ELFCLASS64 = elf.ELFCLASS64

const PF_X = elf.PF_X
const PF_W = elf.PF_W

const STB_GLOBAL = elf.STB_GLOBAL
const STT_FUNC = elf.STT_FUNC

const SHF_COMPRESSED = elf.SHF_COMPRESSED
