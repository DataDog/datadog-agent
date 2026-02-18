// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//nolint:revive
package safeelf

import "debug/elf" //nolint:depguard

type Prog = elf.Prog
type Symbol = elf.Symbol
type SymType = elf.SymType
type SymBind = elf.SymBind
type Section = elf.Section
type SectionHeader = elf.SectionHeader
type SectionType = elf.SectionType
type SectionIndex = elf.SectionIndex
type SectionFlag = elf.SectionFlag
type Machine = elf.Machine

var ErrNoSymbols = elf.ErrNoSymbols

const SHF_ALLOC = elf.SHF_ALLOC
const SHF_EXECINSTR = elf.SHF_EXECINSTR

const SHT_SYMTAB = elf.SHT_SYMTAB
const SHT_DYNSYM = elf.SHT_DYNSYM
const SHT_NOTE = elf.SHT_NOTE
const SHT_REL = elf.SHT_REL
const SHT_RELA = elf.SHT_RELA //nolint:misspell
const SHT_HASH = elf.SHT_HASH
const SHT_DYNAMIC = elf.SHT_DYNAMIC
const SHT_GNU_HASH = elf.SHT_GNU_HASH
const SHT_GNU_VERDEF = elf.SHT_GNU_VERDEF
const SHT_GNU_VERNEED = elf.SHT_GNU_VERNEED
const SHT_GNU_VERSYM = elf.SHT_GNU_VERSYM

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
const PF_R = elf.PF_R

const STB_GLOBAL = elf.STB_GLOBAL
const STB_WEAK = elf.STB_WEAK
const STT_OBJECT = elf.STT_OBJECT
const STT_FUNC = elf.STT_FUNC
const STT_FILE = elf.STT_FILE
const SHN_UNDEF = elf.SHN_UNDEF
const SHF_WRITE = elf.SHF_WRITE
const SHT_NOBITS = elf.SHT_NOBITS
const SHT_PROGBITS = elf.SHT_PROGBITS
const SHN_ABS = elf.SHN_ABS
const SHN_COMMON = elf.SHN_COMMON

const SHF_COMPRESSED = elf.SHF_COMPRESSED

func ST_TYPE(info uint8) SymType { return elf.ST_TYPE(info) }
func ST_BIND(info uint8) SymBind { return elf.ST_BIND(info) }
