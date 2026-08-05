// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosymname"
)

// Utilities for parsing Go function names.

type parseFuncNameFailureReason int

const (
	parseFuncNameFailureReasonUndefined parseFuncNameFailureReason = iota
	// Functions like time.map.init.0 that initialize statically-defined maps.
	parseFuncNameFailureReasonMapInit
	// parseFuncNameFailureReasonUnsupported covers compiler-generated symbols,
	// assembly stubs, C functions, and other non-Go functions we can't instrument.
	parseFuncNameFailureReasonUnsupported
)

// funcName is the result of parsing a Go function name by parseFuncName().
type funcName struct {
	Package string
	// Type is the type of the receiver, if any. Empty if this function is not a
	// method. The type is not a pointer type even if the method has a
	// pointer-receiver; the base type is returned, without the '*'.
	// For generic types, the type parameters are replaced with [...], e.g.
	// "typeWithGenerics[...]".
	Type string
	Name string
	// QualifiedName is the canonical name used to identify the function for
	// probing. For generic functions, type parameters are replaced with [...],
	// e.g. "main.typeWithGenerics[...].Guess".
	QualifiedName string
}

func (f *funcName) Empty() bool {
	return f.Name == ""
}

// parseFuncNameResult is the result of parsing a Go function name by
// parseFuncName().
type parseFuncNameResult struct {
	// failureReason is set if the function name was not be parsed because the
	// function is not supported. Such functions should be ignored.
	failureReason parseFuncNameFailureReason
	// funcName is the parsed function name. Set if failureReason is not set.
	funcName funcName
}

// parseFuncName parses a Go qualified function name using the gosymname
// library. For a qualifiedName like:
// github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker
// the package is: github.com/cockroachdb/cockroach/pkg/kv/kvserver
// the type is: raftSchedulerShard
// the name is: worker
//
// Generic type parameters are canonicalized to [...], e.g.
// main.typeWithGenerics[go.shape.int].Guess becomes
// main.typeWithGenerics[...].Guess.
//
// Some functions are not supported. For these, failureReason is set on the
// result and a nil error is returned.
func parseFuncName(qualifiedName string) (parseFuncNameResult, error) {
	sym := gosymname.Parse(qualifiedName, gosymname.SourceDWARF)

	switch sym.Class() {
	case gosymname.ClassMapInit:
		return parseFuncNameResult{
			failureReason: parseFuncNameFailureReasonMapInit,
		}, nil
	case gosymname.ClassCompilerGenerated, gosymname.ClassBareName, gosymname.ClassCFunction:
		// Not a Go function we should index.
		return parseFuncNameResult{
			failureReason: parseFuncNameFailureReasonUnsupported,
		}, nil
	case gosymname.ClassGlobalClosure:
		// glob.funcN — not interesting for probing.
		return parseFuncNameResult{
			failureReason: parseFuncNameFailureReasonUnsupported,
		}, nil
	}

	interps := sym.Interpretations()
	if len(interps) == 0 {
		return parseFuncNameResult{
			failureReason: parseFuncNameFailureReasonUnsupported,
		}, nil
	}

	// Find the best interpretation, preferring the method interpretation
	// for backward compatibility with the old regex-based parser which
	// treated any "pkg.Type.Name" as a value-receiver method.
	best := findPreferredInterpretation(interps)

	canonicalQName := gosymname.CanonicalizeGenerics(qualifiedName)

	// Determine the receiver type (if any) and the display name.
	// The old parser's behavior:
	// - Simple method (no closures/inlined): Type=receiver, Name=method
	// - Method with closures/inlined: Type="", Name="receiver.method.func1..."
	// - Function (no receiver): Type="", Name=Local()
	//
	// We replicate this by checking: if there's a receiver AND the local name
	// has no dots beyond the "Type.Method" boundary (i.e. no closures, no
	// inlined), keep Type separate. Otherwise, collapse everything into Name.

	var typ, name string
	local := sym.Local()
	canonicalLocal := gosymname.CanonicalizeGenerics(local)

	if best.IsMethod() {
		// Construct the receiver type (without '*', with canonicalized generics).
		recvType := best.OuterReceiver
		if best.OuterReceiverGenerics != nil {
			recvType += "[...]"
		}

		// Check if this is a "simple" method (no closures, no inlined calls).
		if !best.HasInlinedCalls() && best.ClosureSuffix == "" {
			typ = recvType
			name = best.OuterFunction
		} else {
			// Complex case: collapse receiver into name.
			// The old parser's behavior: include the bare receiver type name
			// (without '*' or parens) as part of Name.
			// E.g. "(*apiV2Server).execSQL.func8" → "apiV2Server.execSQL.func8"
			typ = ""
			name = collapseReceiverIntoName(canonicalLocal)
		}
	} else {
		typ = ""
		name = canonicalLocal
	}

	return parseFuncNameResult{
		funcName: funcName{
			Package:       sym.Package(),
			Type:          typ,
			Name:          name,
			QualifiedName: canonicalQName,
		},
	}, nil
}

// findPreferredInterpretation selects the interpretation to use for symdb.
// When there are multiple interpretations (ambiguous symbol), prefer the
// method interpretation (value-receiver) over the function+inlined
// interpretation. This matches the old regex parser behavior which treated
// "pkg.Type.Name" as a value-receiver method. When there's a closure suffix,
// prefer the non-method interpretation (the old parser collapsed these).
func findPreferredInterpretation(interps []gosymname.Interpretation) *gosymname.Interpretation {
	if len(interps) == 1 {
		return &interps[0]
	}

	// For ambiguous symbols with closures, prefer the non-method
	// interpretation — the old parser treated these as functions, not methods.
	hasClosure := false
	for i := range interps {
		if interps[i].ClosureSuffix != "" {
			hasClosure = true
			break
		}
	}
	if hasClosure {
		// Prefer non-method interpretation.
		for i := range interps {
			if !interps[i].IsMethod() {
				return &interps[i]
			}
		}
	}

	// For simple ambiguous symbols (no closures), prefer the method
	// interpretation — the old parser matched these as value-receiver methods.
	for i := range interps {
		if interps[i].IsMethod() {
			return &interps[i]
		}
	}

	return &interps[0]
}

// collapseReceiverIntoName transforms a local name by stripping the pointer
// receiver syntax but keeping the type name. For example:
//
//	"(*apiV2Server).execSQL.func8" → "apiV2Server.execSQL.func8"
//	"(*Type[...]).Method.func1"   → "Type[...].Method.func1"
//	"Foo.Bar.func1"               → "Foo.Bar.func1" (unchanged, no ptr syntax)
func collapseReceiverIntoName(local string) string {
	if !strings.HasPrefix(local, "(*") {
		return local
	}
	// Find matching ')' then '.'
	closeIdx := strings.IndexByte(local, ')')
	if closeIdx == -1 || closeIdx+1 >= len(local) || local[closeIdx+1] != '.' {
		return local
	}
	// Extract the type name between (* and ), then append the rest.
	typeName := local[2:closeIdx]
	rest := local[closeIdx+2:]
	return typeName + "." + rest
}

// splitPkg splits a full linker symbol name into package (full import path) and
// local symbol name.
//
// Copied from
// https://github.com/golang/go/blob/c0025d5e0b3f6fca7117e9b8f4593a95e37a9fa5/src/cmd/compile/internal/ir/func.go#L367
func splitPkg(name string) (pkgpath, sym string) {
	// package-sym split is at first dot after last the / that comes before
	// any characters illegal in a package path.

	lastSlashIdx := 0
	for i, r := range name {
		// Catches cases like:
		// * example.foo[sync/atomic.Uint64].
		// * example%2ecom.foo[sync/atomic.Uint64].
		//
		// Note that name is still escaped; unescape occurs after splitPkg.
		if !escapedImportPathOK(r) {
			break
		}
		if r == '/' {
			lastSlashIdx = i
		}
	}
	for i := lastSlashIdx; i < len(name); i++ {
		r := name[i]
		if r == '.' {
			return name[:i], name[i+1:]
		}
	}

	return "", name
}

// parseLinkFuncName parsers a symbol name (such as a type or function name) as
// it appears in DWARF to the package path and local identifier name. The
// returned package name is unescaped. If the package name contained escape
// sequences, wasEscaped is returned true. Otherwise, name == <pkg>.<sym>
//
// This and related functions were adapted from
// https://github.com/golang/go/blob/7a1679d7ae32dd8a01bd355413ee77ba517f5f43/src/cmd/internal/objabi/path.go#L18
func parseLinkFuncName(name string) (pkg, sym string, wasEscaped bool, err error) {
	pkg, sym = splitPkg(name)
	if pkg == "" {
		return "", sym, false, nil
	}

	pkg, wasEscaped, err = prefixToPath(pkg) // unescape
	if err != nil {
		return "", "", false, fmt.Errorf("malformed package path: %v", err)
	}

	return pkg, sym, wasEscaped, nil
}

// unescapeSymbol takes a symbol name as it appears in DWARF (i.e. package
// import path + identifier) and unescapes the package import path, returning
// the full symbol name with the unescaped package path.
func unescapeSymbol(name string) (string, error) {
	pkg, sym, wasEscaped, err := parseLinkFuncName(name)
	if err != nil {
		return "", err
	}
	if !wasEscaped {
		// Avoid allocation on the common case.
		return name, nil
	}
	return pkg + "." + sym, nil
}

// prefixToPath unescapes package import paths, replacing escape sequences with
// the original character.
//
// The bool return value is true if any escape sequences were found and
// replaced.
func prefixToPath(s string) (string, bool, error) {
	// Short-circuit the common case.
	percent := strings.IndexByte(s, '%')
	if percent == -1 {
		return s, false, nil
	}

	p := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		if s[i] != '%' {
			p = append(p, s[i])
			i++
			continue
		}
		if i+2 >= len(s) {
			// Not enough characters remaining to be a valid escape
			// sequence.
			return "", false, fmt.Errorf("malformed prefix %q: escape sequence must contain two hex digits", s)
		}

		b, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			// Not a valid escape sequence.
			return "", false, fmt.Errorf("malformed prefix %q: escape sequence %q must contain two hex digits", s, s[i:i+3])
		}

		p = append(p, byte(b))
		i += 3
	}
	return string(p), true, nil
}

func modPathOK(r rune) bool {
	if r < utf8.RuneSelf {
		return r == '-' || r == '.' || r == '_' || r == '~' ||
			'0' <= r && r <= '9' ||
			'A' <= r && r <= 'Z' ||
			'a' <= r && r <= 'z'
	}
	return false
}

func escapedImportPathOK(r rune) bool {
	return modPathOK(r) || r == '+' || r == '/' || r == '%'
}
