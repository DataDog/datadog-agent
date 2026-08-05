// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package gosymname parses Go symbol names — the function names that appear in
// CPU profiles, DWARF debug info, pclntab, and `go tool nm` output — and
// extracts structured information: package, receiver type, function name,
// generic type parameters, closure nesting, inlined calls, and more.
//
// Go symbol names are ambiguous due to the overloaded use of '.' as a
// separator for import paths, package boundaries, receiver types, method
// names, closures, and inlined function chains. This package handles all of
// these cases, returning multiple interpretations when the parse is ambiguous
// and providing confidence scores based on Go naming conventions.
//
// Quick-access methods like Package(), Class(), and IsClosure() avoid full
// parsing. The full parse (chain decomposition and interpretation generation)
// is deferred until Interpretations() is called, and is cached.
package gosymname
