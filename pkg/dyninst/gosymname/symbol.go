// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import "strings"

// Symbol is a parsed Go symbol name. The package split is computed eagerly,
// but full chain decomposition and interpretation generation are deferred
// until requested.
type Symbol struct {
	raw    string
	source SymbolSource

	// Eagerly computed by Parse.
	pkg   string // unescaped package import path
	local string // everything after the package dot
	class SymbolClass

	// Lazily computed by ensureInterps.
	interpsDone bool
	interps     []Interpretation
	interpsBuf  [2]Interpretation // inline storage to avoid heap allocation
}

// Raw returns the original symbol name string.
func (s *Symbol) Raw() string { return s.raw }

// Package returns the unescaped import path of the outer function's package.
func (s *Symbol) Package() string { return s.pkg }

// Local returns everything after the package dot separator.
func (s *Symbol) Local() string { return s.local }

// Class returns the coarse symbol classification.
func (s *Symbol) Class() SymbolClass { return s.class }

// IsGeneric returns true if the symbol contains '[' (generic type parameters).
func (s *Symbol) IsGeneric() bool {
	return strings.ContainsRune(s.raw, '[')
}

// IsCompilerGenerated returns true if this is a compiler-generated symbol.
func (s *Symbol) IsCompilerGenerated() bool {
	return s.class == ClassCompilerGenerated
}

// IsClosure returns true if the symbol represents a closure.
func (s *Symbol) IsClosure() bool {
	return s.class == ClassClosure || s.class == ClassGlobalClosure
}

// HasPointerReceiver returns true if the local name contains "(*".
func (s *Symbol) HasPointerReceiver() bool {
	return strings.Contains(s.local, "(*")
}

// Interpretations returns all valid structural readings of this symbol.
// The result is cached after the first call.
func (s *Symbol) Interpretations() []Interpretation {
	s.ensureInterps()
	return s.interps
}

// IsAmbiguous returns true if the symbol has more than one valid
// interpretation.
func (s *Symbol) IsAmbiguous() bool {
	s.ensureInterps()
	return len(s.interps) > 1
}

func (s *Symbol) ensureInterps() {
	if s.interpsDone {
		return
	}
	s.interpsDone = true
	s.interps = s.interpsBuf[:0]
	buildInterpretations(s)
}
