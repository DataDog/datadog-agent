// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import (
	"strings"
)

// Parse parses a Go symbol name and returns a Symbol with the package split
// computed eagerly and interpretation generation deferred.
func Parse(name string, source SymbolSource) Symbol {
	var s Symbol
	ParseInto(&s, name, source)
	return s
}

// ParseInto parses a Go symbol name into an existing Symbol, avoiding
// allocation of the Symbol struct itself.
func ParseInto(dst *Symbol, name string, source SymbolSource) {
	*dst = Symbol{
		raw:    name,
		source: source,
	}

	// Strip ABI suffix for nm source before classification.
	effective := name
	if source.hasABISuffix() {
		effective, _ = stripABISuffix(name)
	}

	// Quick classification before full split.
	dst.class = classify(effective, source)
	if dst.class == ClassCompilerGenerated {
		dst.pkg = ""
		dst.local = effective
		if strings.HasPrefix(effective, "go:") || strings.HasPrefix(effective, "type:") || strings.HasPrefix(effective, "type..") {
			dst.local = effective[strings.IndexByte(effective, ':')+1:]
		}
		return
	}
	if dst.class == ClassBareName {
		dst.pkg = ""
		dst.local = effective
		return
	}
	if dst.class == ClassCFunction {
		dst.pkg = ""
		dst.local = effective
		return
	}

	// Split package from local name.
	escapedPkg, local := splitPkg(effective)
	if escapedPkg == "" {
		// No package found — treat as bare name.
		dst.pkg = ""
		dst.local = effective
		if dst.class != ClassGlobalClosure {
			dst.class = ClassBareName
		}
		return
	}

	// Unescape the package path.
	pkg, err := unescapePkg(escapedPkg)
	if err != nil {
		// Malformed escape — best effort, use raw.
		pkg = escapedPkg
	}
	dst.pkg = pkg
	dst.local = local

	// Handle global closure (glob.funcN).
	if pkg == "glob" {
		dst.class = ClassGlobalClosure
		return
	}

	// Re-classify based on local name now that we have it.
	dst.class = classifyLocal(local, dst.class)
}

// SplitPackage splits a symbol name into its package path and local name.
// The package path is unescaped.
func SplitPackage(name string, source SymbolSource) (pkg, local string) {
	effective := name
	if source.hasABISuffix() {
		effective, _ = stripABISuffix(name)
	}
	escapedPkg, local := splitPkg(effective)
	if escapedPkg == "" {
		return "", effective
	}
	pkg, err := unescapePkg(escapedPkg)
	if err != nil {
		return escapedPkg, local
	}
	return pkg, local
}

// stripABISuffix removes a trailing .abi0 or .abiinternal suffix and returns
// the name without the suffix and the suffix itself (without the leading dot).
func stripABISuffix(name string) (stripped, suffix string) {
	if strings.HasSuffix(name, ".abi0") {
		return name[:len(name)-5], "abi0"
	}
	if strings.HasSuffix(name, ".abiinternal") {
		return name[:len(name)-12], "abiinternal"
	}
	return name, ""
}

// classify performs quick classification of a symbol name.
func classify(name string, source SymbolSource) SymbolClass {
	// Compiler-generated prefixes — check first 5 bytes to cover "go:",
	// "type:", "type..", "glob.".
	if len(name) >= 3 {
		switch {
		case name[0] == 'g' && name[1] == 'o' && name[2] == ':':
			return ClassCompilerGenerated
		case name[0] == 'g' && name[1] == 'l' && len(name) >= 5 && name[2:5] == "ob.":
			return ClassGlobalClosure
		case name[0] == 't' && len(name) >= 5 && name[1:5] == "ype:" || name[0] == 't' && len(name) >= 6 && name[1:6] == "ype..":
			return ClassCompilerGenerated
		}
	}

	// Single pass: find '.', '/', and '..' to determine class.
	hasDot := false
	hasSlash := false
	hasDoubleDot := false
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '.':
			hasDot = true
			if i+1 < len(name) && name[i+1] == '.' {
				hasDoubleDot = true
			}
		case '/':
			hasSlash = true
		}
	}

	// Compiler-generated infixes all contain "..".
	if hasDoubleDot {
		if strings.Contains(name, "..inittask") || strings.Contains(name, "..stmp_") || strings.Contains(name, "..dict.") {
			return ClassCompilerGenerated
		}
	}

	// C function heuristic: no '/' and has GCC optimization suffixes.
	if !hasSlash && hasDot {
		if strings.Contains(name, ".isra.") || strings.Contains(name, ".part.") || strings.Contains(name, ".constprop.") {
			return ClassCFunction
		}
	}

	// Bare name: no '.' and no '/'.
	if !hasDot && !hasSlash {
		return ClassBareName
	}

	_ = source
	return ClassFunction
}

// classifyLocal refines classification based on the local name (after package
// split).
func classifyLocal(local string, current SymbolClass) SymbolClass {
	// Map init: map.init.N
	if strings.HasPrefix(local, "map.init.") {
		return ClassMapInit
	}

	// Init function: init or init.N
	if local == "init" || strings.HasPrefix(local, "init.") {
		allDigits := true
		suffix := ""
		if len(local) > 5 {
			suffix = local[5:]
		}
		for _, c := range suffix {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if local == "init" || (suffix != "" && allDigits) {
			return ClassInit
		}
	}

	// Closure detection: .funcN, .gowrapN, .deferwrapN, -rangeN anywhere.
	if hasClosure(local) {
		return ClassClosure
	}

	return current
}

// hasClosure returns true if the local name contains closure markers.
func hasClosure(local string) bool {
	for i := 0; i < len(local); i++ {
		if local[i] == '.' {
			rest := local[i+1:]
			if matchPrefix(rest, "func") || matchPrefix(rest, "gowrap") || matchPrefix(rest, "deferwrap") {
				return true
			}
		}
		if local[i] == '-' {
			rest := local[i+1:]
			if matchPrefix(rest, "range") {
				return true
			}
		}
	}
	return false
}

// matchPrefix checks if s starts with prefix followed by a digit.
func matchPrefix(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	rest := s[len(prefix):]
	if len(rest) == 0 {
		// "deferwrap" with no number is still valid
		return prefix == "deferwrap" || prefix == "gowrap"
	}
	return rest[0] >= '0' && rest[0] <= '9'
}

// segment types used during chain decomposition.
type segmentKind int8

const (
	segName    segmentKind = iota // plain identifier
	segPtrRecv                    // (*Type).Method
	segClosure                    // funcN, gowrapN, deferwrapN
	segNesting                    // bare number (closure nesting)
	segRange                      // -rangeN
	segFM                         // -fm
)

// segment is a compact representation of a parsed segment. It uses a
// union-style layout: two name+generics pairs whose meaning depends on kind.
//
// For segPtrRecv: ident1 = receiver, ident2 = method.
// For segName: ident1 = the identifier, ident2 is unused.
// For other kinds: only text is used.
type segment struct {
	kind   segmentKind
	text   string // raw text of this segment
	ident1 nameAndGenerics
	ident2 nameAndGenerics
}

type nameAndGenerics struct {
	name string
	gen  *GenericParams
}

// Accessors that give meaningful names at call sites.
func (s *segment) receiver() string             { return s.ident1.name }
func (s *segment) receiverGen() *GenericParams  { return s.ident1.gen }
func (s *segment) method() string               { return s.ident2.name }
func (s *segment) methodGen() *GenericParams    { return s.ident2.gen }
func (s *segment) funcName() string             { return s.ident1.name }
func (s *segment) funcGenerics() *GenericParams { return s.ident1.gen }

const maxInlineSegments = 8

// buildInterpretations performs full chain decomposition and generates
// interpretations for the symbol.
func buildInterpretations(s *Symbol) {
	switch s.class {
	case ClassCompilerGenerated:
		s.interps = append(s.interps, Interpretation{
			OuterFunction: s.raw,
		})
		return
	case ClassBareName,
		ClassCFunction,
		ClassGlobalClosure,
		ClassMapInit,
		ClassInit:
		s.interps = append(s.interps, Interpretation{
			OuterFunction: s.local,
		})
		return
	case ClassFunction, ClassClosure:
		break
	}

	// Decompose the local name into segments.
	var segBuf [maxInlineSegments]segment
	segments := decomposeChain(segBuf[:0], s.local)
	if len(segments) == 0 {
		s.interps = append(s.interps, Interpretation{
			OuterFunction: s.local,
		})
		return
	}

	// Determine ABI suffix.
	abiSuffix := ""
	if s.source.hasABISuffix() {
		_, abiSuffix = stripABISuffix(s.raw)
	}

	// Separate trailing closure/wrapper segments from the function chain.
	var chainBuf [maxInlineSegments]segment
	chainSegs, closureSuffix, closureDepth, wrapper := splitClosureChain(chainBuf[:0], segments)

	if len(chainSegs) == 0 {
		// Only closure segments — shouldn't happen with a valid symbol, but
		// handle gracefully.
		s.interps = append(s.interps, Interpretation{
			OuterFunction: s.local,
			ClosureSuffix: closureSuffix,
			ClosureDepth:  closureDepth,
			Wrapper:       wrapper,
			ABISuffix:     abiSuffix,
		})
		return
	}

	// Method expression wrappers (-fm) are unambiguous: the first segment is
	// always a value receiver, the second is the method.
	if wrapper == WrapperMethodExpr && len(chainSegs) >= 2 {
		first := &chainSegs[0]
		second := &chainSegs[1]
		recvKind := ReceiverValue
		if first.kind == segPtrRecv {
			recvKind = ReceiverPointer
		}
		interp := Interpretation{
			OuterReceiverKind:     recvKind,
			OuterReceiverGenerics: first.receiverGen(),
			Wrapper:               WrapperMethodExpr,
			ABISuffix:             abiSuffix,
		}
		if first.kind == segPtrRecv {
			interp.OuterReceiver = first.receiver()
			interp.OuterFunction = first.method()
			interp.OuterFuncGenerics = first.methodGen()
		} else {
			interp.OuterReceiver = first.funcName()
			interp.OuterFunction = second.funcName()
			interp.OuterFuncGenerics = second.funcGenerics()
		}
		interp.InlinedCalls = buildInlinedCalls(chainSegs[2:])
		s.interps = append(s.interps, interp)
		return
	}

	// Build interpretations from the chain segments.
	// The first segment determines whether we have a receiver ambiguity.
	first := &chainSegs[0]

	switch first.kind {
	case segPtrRecv:
		// Unambiguous pointer-receiver method.
		interp := Interpretation{
			OuterReceiver:         first.receiver(),
			OuterReceiverKind:     ReceiverPointer,
			OuterReceiverGenerics: first.receiverGen(),
			OuterFunction:         first.method(),
			OuterFuncGenerics:     first.methodGen(),
			ClosureSuffix:         closureSuffix,
			ClosureDepth:          closureDepth,
			Wrapper:               wrapper,
			ABISuffix:             abiSuffix,
		}
		// Remaining chain segments are inlined calls.
		interp.InlinedCalls = buildInlinedCalls(chainSegs[1:])
		s.interps = append(s.interps, interp)

	case segName:
		if len(chainSegs) == 1 {
			// Single name segment — unambiguous function.
			s.interps = append(s.interps, Interpretation{
				OuterFunction:     first.funcName(),
				OuterFuncGenerics: first.funcGenerics(),
				ClosureSuffix:     closureSuffix,
				ClosureDepth:      closureDepth,
				Wrapper:           wrapper,
				ABISuffix:         abiSuffix,
			})
		} else {
			second := &chainSegs[1]
			if second.kind == segPtrRecv {
				// First is a function name, second is an unambiguous ptr-recv
				// inlined method. No ambiguity about the first segment.
				interp := Interpretation{
					OuterFunction:     first.funcName(),
					OuterFuncGenerics: first.funcGenerics(),
					ClosureSuffix:     closureSuffix,
					ClosureDepth:      closureDepth,
					Wrapper:           wrapper,
					ABISuffix:         abiSuffix,
				}
				interp.InlinedCalls = buildInlinedCalls(chainSegs[1:])
				s.interps = append(s.interps, interp)
			} else {
				// Ambiguous: first could be a value receiver or a function
				// name. Generate both interpretations.

				// Interp 1: first is a function, rest are inlined.
				interp1 := Interpretation{
					OuterFunction:     first.funcName(),
					OuterFuncGenerics: first.funcGenerics(),
					ClosureSuffix:     closureSuffix,
					ClosureDepth:      closureDepth,
					Wrapper:           wrapper,
					ABISuffix:         abiSuffix,
				}
				interp1.InlinedCalls = buildInlinedCalls(chainSegs[1:])

				// Interp 2: first is a value receiver, second is the method.
				interp2 := Interpretation{
					OuterReceiver:         first.funcName(),
					OuterReceiverKind:     ReceiverValue,
					OuterReceiverGenerics: first.funcGenerics(),
					ClosureSuffix:         closureSuffix,
					ClosureDepth:          closureDepth,
					Wrapper:               wrapper,
					ABISuffix:             abiSuffix,
				}
				switch second.kind {
				case segName:
					interp2.OuterFunction = second.funcName()
					interp2.OuterFuncGenerics = second.funcGenerics()
					interp2.InlinedCalls = buildInlinedCalls(chainSegs[2:])
				case segPtrRecv:
					// Shouldn't happen (handled above), but be safe.
					interp2.OuterFunction = second.method()
					interp2.OuterFuncGenerics = second.methodGen()
					interp2.InlinedCalls = buildInlinedCalls(chainSegs[2:])
				}

				s.interps = append(s.interps, interp1, interp2)
			}
		}
	}
}

// decomposeChain breaks the local name into a sequence of segments by scanning
// left-to-right, handling brackets, pointer receivers, closures, and nesting.
// The dst slice should be backed by a caller-owned array to avoid heap
// allocation for small segment counts.
func decomposeChain(dst []segment, local string) []segment {
	segments := dst
	i := 0
	for i < len(local) {
		// Skip leading dots between segments.
		if local[i] == '.' {
			i++
			continue
		}

		// Case 1: Pointer receiver (*Type).Method
		if i < len(local)-1 && local[i] == '(' && local[i+1] == '*' {
			seg, end := parsePtrRecvSegment(local, i)
			if end > i {
				segments = append(segments, seg)
				i = end
				continue
			}
		}

		// Case 2: Check for -range, -fm suffixes.
		if local[i] == '-' {
			rest := local[i+1:]
			if strings.HasPrefix(rest, "range") {
				// Find end of rangeN
				end := i + 1 + 5 // past "range"
				for end < len(local) && local[end] >= '0' && local[end] <= '9' {
					end++
				}
				segments = append(segments, segment{
					kind: segRange,
					text: local[i+1 : end],
				})
				i = end
				continue
			}
			if strings.HasPrefix(rest, "fm") && (i+3 >= len(local) || local[i+3] == '.') {
				segments = append(segments, segment{
					kind: segFM,
					text: "fm",
				})
				i += 3
				continue
			}
		}

		// Case 3: Closure markers (funcN, gowrapN, deferwrapN).
		if kind, end, ok := tryParseClosure(local, i); ok {
			segments = append(segments, segment{
				kind: kind,
				text: local[i:end],
			})
			i = end
			continue
		}

		// Case 4: Bare number (closure nesting).
		if local[i] >= '0' && local[i] <= '9' {
			end := i
			for end < len(local) && local[end] >= '0' && local[end] <= '9' {
				end++
			}
			segments = append(segments, segment{
				kind: segNesting,
				text: local[i:end],
			})
			i = end
			continue
		}

		// Case 5: Name segment — scan to next dot (outside brackets).
		name, gen, end := parseNameSegment(local, i)
		segments = append(segments, segment{
			kind:   segName,
			text:   local[i:end],
			ident1: nameAndGenerics{name: name, gen: gen},
		})
		i = end
	}
	return segments
}

// parsePtrRecvSegment parses a (*Type[Generics]).Method[Generics] segment.
// Returns the segment and the position after the segment.
func parsePtrRecvSegment(local string, start int) (segment, int) {
	// local[start] == '(', local[start+1] == '*'
	i := start + 2

	// Find the receiver name (and optional generics).
	recvStart := i
	recvName := ""
	var recvGen *GenericParams

	for i < len(local) {
		if local[i] == '[' {
			recvName = local[recvStart:i]
			bracketEnd := MatchBracket(local, i)
			if bracketEnd == -1 {
				return segment{}, start
			}
			recvGen = &GenericParams{
				Raw:   local[i+1 : bracketEnd],
				Start: i,
				End:   bracketEnd,
			}
			i = bracketEnd + 1
			break
		}
		if local[i] == ')' {
			recvName = local[recvStart:i]
			break
		}
		i++
	}

	// Expect ')' then '.'
	if i >= len(local) || local[i] != ')' {
		return segment{}, start
	}
	i++ // skip ')'
	if i >= len(local) || local[i] != '.' {
		return segment{}, start
	}
	i++ // skip '.'

	// Parse the method name (and optional generics).
	methStart := i
	methName := ""
	var methGen *GenericParams

	for i < len(local) {
		if local[i] == '[' {
			methName = local[methStart:i]
			bracketEnd := MatchBracket(local, i)
			if bracketEnd == -1 {
				return segment{}, start
			}
			methGen = &GenericParams{
				Raw:   local[i+1 : bracketEnd],
				Start: i,
				End:   bracketEnd,
			}
			i = bracketEnd + 1
			break
		}
		if local[i] == '.' || local[i] == '-' {
			methName = local[methStart:i]
			break
		}
		i++
	}
	if methName == "" {
		methName = local[methStart:i]
	}

	seg := segment{
		kind:   segPtrRecv,
		text:   local[start:i],
		ident1: nameAndGenerics{name: recvName, gen: recvGen},
		ident2: nameAndGenerics{name: methName, gen: methGen},
	}
	return seg, i
}

// tryParseClosure attempts to parse a closure marker (funcN, gowrapN,
// deferwrapN) at position start. Returns the segment kind, end position, and
// whether it matched.
func tryParseClosure(local string, start int) (segmentKind, int, bool) {
	rest := local[start:]

	for _, prefix := range [...]string{"func", "gowrap", "deferwrap"} {
		if !strings.HasPrefix(rest, prefix) {
			continue
		}
		end := start + len(prefix)
		// Must be followed by a digit, or for gowrap/deferwrap, end of string
		// or dot is also ok.
		if end < len(local) && local[end] >= '0' && local[end] <= '9' {
			for end < len(local) && local[end] >= '0' && local[end] <= '9' {
				end++
			}
			return segClosure, end, true
		}
		if (prefix == "gowrap" || prefix == "deferwrap") && (end >= len(local) || local[end] == '.') {
			return segClosure, end, true
		}
	}
	return 0, 0, false
}

// parseNameSegment parses an identifier segment, potentially with generic
// parameters. Returns the name, optional GenericParams, and end position.
func parseNameSegment(local string, start int) (string, *GenericParams, int) {
	i := start
	for i < len(local) {
		if local[i] == '[' {
			name := local[start:i]
			bracketEnd := MatchBracket(local, i)
			if bracketEnd == -1 {
				// Unmatched bracket — treat rest as the name.
				return local[start:], nil, len(local)
			}
			gen := &GenericParams{
				Raw:   local[i+1 : bracketEnd],
				Start: i,
				End:   bracketEnd,
			}
			return name, gen, bracketEnd + 1
		}
		if local[i] == '.' || local[i] == '-' {
			return local[start:i], nil, i
		}
		i++
	}
	return local[start:i], nil, i
}

// splitClosureChain separates trailing closure/nesting/wrapper segments from
// the function/method chain, handling interleaved closure and inlined
// segments.
func splitClosureChain(dst []segment, segments []segment) (chain []segment, closureSuffix string, closureDepth int, wrapper WrapperKind) {
	// Find the first closure/nesting/range/fm segment. Everything before it
	// that is a name or ptr-recv is part of the chain. After the first
	// closure, we split: closure/nesting goes to suffix, but ptr-recv and
	// name segments between closures go to the chain as inlined calls.

	firstClosure := -1
	for i := range segments {
		kind := segments[i].kind
		if kind == segClosure || kind == segNesting || kind == segRange || kind == segFM {
			firstClosure = i
			break
		}
	}

	if firstClosure == -1 {
		// No closure segments at all.
		return segments, "", 0, WrapperNone
	}

	// Everything before the first closure is unambiguously the chain.
	// Use the caller-provided buffer for the chain slice.
	chain = append(dst, segments[:firstClosure]...)

	// Walk the rest, building the closure suffix directly with a builder
	// to avoid []string + strings.Join.
	var sb strings.Builder
	suffixParts := 0
	for idx := firstClosure; idx < len(segments); idx++ {
		seg := &segments[idx]
		switch seg.kind {
		case segClosure:
			if suffixParts > 0 {
				sb.WriteByte('.')
			}
			sb.WriteString(seg.text)
			suffixParts++
			// gowrap and deferwrap are wrappers, not additional closure
			// nesting levels.
			if !strings.HasPrefix(seg.text, "gowrap") && !strings.HasPrefix(seg.text, "deferwrap") {
				closureDepth++
			} else if strings.HasPrefix(seg.text, "gowrap") {
				wrapper = WrapperGoWrap
			} else {
				wrapper = WrapperDeferWrap
			}
		case segNesting:
			if suffixParts > 0 {
				sb.WriteByte('.')
			}
			sb.WriteString(seg.text)
			suffixParts++
			closureDepth++
		case segRange:
			// Range attaches to the last closure suffix part with '-'.
			if suffixParts > 0 {
				sb.WriteByte('-')
			}
			sb.WriteString(seg.text)
		case segFM:
			wrapper = WrapperMethodExpr
		case segPtrRecv, segName:
			// Inlined call interleaved with closures.
			chain = append(chain, *seg)
		}
	}

	closureSuffix = sb.String()
	return chain, closureSuffix, closureDepth, wrapper
}

// buildInlinedCalls converts a slice of chain segments into InlinedCall
// structs.
func buildInlinedCalls(segments []segment) []InlinedCall {
	if len(segments) == 0 {
		return nil
	}
	calls := make([]InlinedCall, 0, len(segments))
	for i := range segments {
		seg := &segments[i]
		switch seg.kind {
		case segPtrRecv:
			calls = append(calls, InlinedCall{
				Receiver:         seg.receiver(),
				HasReceiver:      true,
				ReceiverGenerics: seg.receiverGen(),
				Function:         seg.method(),
				FuncGenerics:     seg.methodGen(),
				Raw:              seg.text,
			})
		case segName:
			calls = append(calls, InlinedCall{
				Function:     seg.funcName(),
				FuncGenerics: seg.funcGenerics(),
				Raw:          seg.text,
			})
		}
	}
	return calls
}
