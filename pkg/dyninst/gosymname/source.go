// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

// SymbolSource indicates the origin of a symbol name, which determines what
// escaping conventions are in effect.
type SymbolSource uint8

const (
	// SourceUnknown is the default; best-effort parsing.
	SourceUnknown SymbolSource = iota
	// SourceDWARF indicates the symbol comes from DWARF debug info. Dots in
	// the last package path segment are escaped as %2e.
	SourceDWARF
	// SourcePclntab indicates the symbol comes from the Go runtime pclntab.
	// Same escaping as DWARF.
	SourcePclntab
	// SourcePprof indicates the symbol comes from a pprof profile. Usually
	// same escaping as pclntab.
	SourcePprof
	// SourceNM indicates the symbol comes from `go tool nm` output. No
	// escaping; may have ABI suffixes (.abi0, .abiinternal).
	SourceNM
)

func (s SymbolSource) hasABISuffix() bool {
	return s == SourceNM
}
