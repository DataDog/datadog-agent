// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package tr implements the tr builtin command.
//
// tr — translate, squeeze, and/or delete characters
//
// Usage: tr [OPTION]... SET1 [SET2]
//
// Translate, squeeze, and/or delete characters from standard input,
// writing to standard output.
//
// Accepted flags:
//
//	-d, --delete
//	    Delete characters in SET1; do not translate.
//
//	-s, --squeeze-repeats
//	    Replace each sequence of a repeated character that is listed
//	    in the last specified SET with a single occurrence of that
//	    character.
//
//	-c, -C, --complement
//	    Use the complement of SET1 (all byte values NOT in SET1).
//
//	-t, --truncate-set1
//	    First truncate SET1 to the length of SET2 when translating.
//
//	-h, --help
//	    Print this usage message to stdout and exit 0.
//
// SET notation:
//
//	Ranges:             a-z, A-Z, 0-9
//	Character classes:  [:alnum:], [:alpha:], [:blank:], [:cntrl:],
//	                    [:digit:], [:graph:], [:lower:], [:upper:],
//	                    [:print:], [:punct:], [:space:], [:xdigit:]
//	Equivalence class:  [=c=]  (matches the literal character c)
//	Repeat:             [c*n]  (repeat char c, n times; [c*] fills, SET2 only)
//	Backslash escapes:  \a \b \f \n \r \t \v \\ \NNN (octal)
//
// Exit codes:
//
//	0  Success.
//	1  Error (invalid arguments, read error, etc.).
//
// Memory safety:
//
//	tr operates on a byte-at-a-time basis using a 256-entry lookup
//	table. Input is read in fixed-size chunks (32 KiB). No allocation
//	is proportional to input size. All loops check ctx.Err() to honour
//	the shell's execution timeout.
package tr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the tr builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "tr",
	Description: "translate or delete characters",
	MakeFlags:   registerFlags,
}

const readBufSize = 32 * 1024

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	fs.SetInterspersed(false)

	help := fs.BoolP("help", "h", false, "print usage and exit")
	deleteFlag := fs.BoolP("delete", "d", false, "delete characters in SET1")
	squeeze := fs.BoolP("squeeze-repeats", "s", false, "squeeze repeated characters")
	complement := fs.BoolP("complement", "c", false, "use complement of SET1")
	var bigC bool
	fs.BoolVarP(&bigC, "complement-alt", "C", false, "alias for -c/--complement")
	_ = fs.MarkHidden("complement-alt")
	truncateSet1 := fs.BoolP("truncate-set1", "t", false, "truncate SET1 to length of SET2")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			callCtx.Out("Usage: tr [OPTION]... SET1 [SET2]\n")
			callCtx.Out("Translate, squeeze, and/or delete characters from standard input,\n")
			callCtx.Out("writing to standard output.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		if bigC {
			*complement = true
		}

		operands := args

		if len(operands) == 0 {
			callCtx.Errf("tr: missing operand\n")
			return builtins.Result{Code: 1}
		}

		if *deleteFlag && *squeeze && len(operands) < 2 {
			callCtx.Errf("tr: missing operand after '%s'\nTwo strings must be given when both deleting and squeezing repeats.\n", builtins.SafeOperand(operands[0]))
			return builtins.Result{Code: 1}
		}

		if *deleteFlag && !*squeeze && len(operands) > 1 {
			callCtx.Errf("tr: extra operand '%s'\nOnly one string may be given when deleting without squeezing repeats.\n", builtins.SafeOperand(operands[1]))
			return builtins.Result{Code: 1}
		}

		if !*deleteFlag && len(operands) < 2 && !*squeeze {
			callCtx.Errf("tr: missing operand after '%s'\n", builtins.SafeOperand(operands[0]))
			return builtins.Result{Code: 1}
		}

		if len(operands) > 2 {
			callCtx.Errf("tr: extra operand '%s'\n", builtins.SafeOperand(operands[2]))
			return builtins.Result{Code: 1}
		}

		set1Str := operands[0]
		var set2Str string
		if len(operands) > 1 {
			set2Str = operands[1]
		}

		var set1Classes []caseClassPos
		var set1ContainsCharClass bool
		set1, err := expandSet(set1Str, false, 0, false, callCtx, &set1Classes, nil, &set1ContainsCharClass)
		if err != nil {
			callCtx.Errf("tr: %s\n", err)
			return builtins.Result{Code: 1}
		}

		if *complement {
			set1 = complementSet(set1)
			set1Classes = nil // complement invalidates class positions
		}

		var set2 []byte
		var set2Classes []caseClassPos
		var set2EndsWithClass bool
		translateMode := !*deleteFlag && len(operands) >= 2
		if set2Str != "" || len(operands) > 1 {
			set2, err = expandSet(set2Str, true, len(set1), translateMode, callCtx, &set2Classes, &set2EndsWithClass, nil)
			if err != nil {
				callCtx.Errf("tr: %s\n", err)
				return builtins.Result{Code: 1}
			}
		}

		if translateMode {
			if !*complement {
				if err := validateCaseClassAlignment(set1Classes, set2Classes); err != nil {
					callCtx.Errf("tr: %s\n", err)
					return builtins.Result{Code: 1}
				}
			}
			if !*truncateSet1 && len(set1) > len(set2) && set2EndsWithClass {
				callCtx.Errf("tr: when translating with string1 longer than string2,\nthe latter string must not end with a character class\n")
				return builtins.Result{Code: 1}
			}
			// GNU tr rejects complemented-class translation when STRING2 doesn't
			// map to a single byte, or when -t is used (truncation makes the
			// full-domain mapping impossible).
			if *complement && set1ContainsCharClass && (*truncateSet1 || !mapsToSingleByte(set2)) {
				callCtx.Errf("tr: when translating with complemented character classes,\nstring2 must map all characters in the domain to one\n")
				return builtins.Result{Code: 1}
			}
		}

		if *deleteFlag {
			if *squeeze {
				return deleteAndSqueeze(ctx, callCtx, set1, set2)
			}
			return deleteBytes(ctx, callCtx, set1)
		}

		if *squeeze && len(operands) == 1 {
			return squeezeOnly(ctx, callCtx, set1)
		}

		if len(operands) >= 2 {
			if !*truncateSet1 && set2Str == "" && len(set1) > 0 {
				callCtx.Errf("tr: when not truncating set1, string2 must be non-empty\n")
				return builtins.Result{Code: 1}
			}
			return translate(ctx, callCtx, set1, set2, *squeeze, *truncateSet1)
		}

		return builtins.Result{}
	}
}

// chunkTransform processes input bytes and appends output to dst, returning
// the updated dst slice.  It is called once per read chunk by processLoop.
type chunkTransform func(dst []byte, src []byte) []byte

// processLoop is the shared I/O loop for all tr modes (delete, squeeze,
// translate, etc.).  It reads stdin in fixed-size chunks, applies transform
// to each chunk, and writes the result to stdout.
func processLoop(ctx context.Context, callCtx *builtins.CallContext, transform chunkTransform) builtins.Result {
	reader := callCtx.Stdin
	if reader == nil {
		return builtins.Result{}
	}

	buf := make([]byte, readBufSize)
	out := make([]byte, 0, readBufSize)
	for {
		if ctx.Err() != nil {
			return builtins.Result{}
		}
		n, readErr := reader.Read(buf)
		if n > 0 {
			out = transform(out[:0], buf[:n])
			if len(out) > 0 {
				if _, werr := callCtx.Stdout.Write(out); werr != nil {
					callCtx.Errf("tr: write error: %s\n", callCtx.PortableErr(werr))
					return builtins.Result{Code: 1}
				}
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				callCtx.Errf("tr: read error: %s\n", callCtx.PortableErr(readErr))
				return builtins.Result{Code: 1}
			}
			return builtins.Result{}
		}
	}
}

func deleteBytes(ctx context.Context, callCtx *builtins.CallContext, set1 []byte) builtins.Result {
	var inSet [256]bool
	for _, b := range set1 {
		inSet[b] = true
	}
	return processLoop(ctx, callCtx, func(dst, src []byte) []byte {
		for _, b := range src {
			if !inSet[b] {
				dst = append(dst, b)
			}
		}
		return dst
	})
}

func deleteAndSqueeze(ctx context.Context, callCtx *builtins.CallContext, set1, set2 []byte) builtins.Result {
	var deleteSet [256]bool
	for _, b := range set1 {
		deleteSet[b] = true
	}
	var squeezeSet [256]bool
	for _, b := range set2 {
		squeezeSet[b] = true
	}
	lastByte := -1
	return processLoop(ctx, callCtx, func(dst, src []byte) []byte {
		for _, b := range src {
			if deleteSet[b] {
				continue
			}
			if squeezeSet[b] && int(b) == lastByte {
				continue
			}
			dst = append(dst, b)
			lastByte = int(b)
		}
		return dst
	})
}

func squeezeOnly(ctx context.Context, callCtx *builtins.CallContext, set1 []byte) builtins.Result {
	var squeezeSet [256]bool
	for _, b := range set1 {
		squeezeSet[b] = true
	}
	lastByte := -1
	return processLoop(ctx, callCtx, func(dst, src []byte) []byte {
		for _, b := range src {
			if squeezeSet[b] && int(b) == lastByte {
				continue
			}
			dst = append(dst, b)
			lastByte = int(b)
		}
		return dst
	})
}

func translate(ctx context.Context, callCtx *builtins.CallContext, set1, set2 []byte, squeeze, truncate bool) builtins.Result {
	if truncate && len(set1) > len(set2) {
		set1 = set1[:len(set2)]
	}

	if !truncate && len(set2) > 0 && len(set1) > len(set2) {
		last := set2[len(set2)-1]
		for len(set2) < len(set1) {
			set2 = append(set2, last)
		}
	}

	var table [256]byte
	for i := range table {
		table[i] = byte(i)
	}
	for i, b := range set1 {
		if i < len(set2) {
			table[b] = set2[i]
		}
	}

	var squeezeSet [256]bool
	if squeeze {
		for _, b := range set2 {
			squeezeSet[b] = true
		}
	}

	lastByte := -1
	return processLoop(ctx, callCtx, func(dst, src []byte) []byte {
		for _, b := range src {
			translated := table[b]
			if squeeze && squeezeSet[translated] && int(translated) == lastByte {
				continue
			}
			dst = append(dst, translated)
			lastByte = int(translated)
		}
		return dst
	})
}

func complementSet(set []byte) []byte {
	var inSet [256]bool
	for _, b := range set {
		inSet[b] = true
	}
	var result []byte
	for i := range 256 {
		if !inSet[byte(i)] {
			result = append(result, byte(i))
		}
	}
	return result
}

func validateCaseClassAlignment(set1Classes, set2Classes []caseClassPos) error {
	if len(set2Classes) == 0 {
		return nil
	}
	s1ByOffset := make(map[int]bool)
	for _, c := range set1Classes {
		s1ByOffset[c.expandedOffset] = true
	}
	for _, c := range set2Classes {
		if !s1ByOffset[c.expandedOffset] {
			return &trError{"misaligned [:upper:] and/or [:lower:] construct"}
		}
	}
	return nil
}

const maxSetLen = 1 << 20

type caseClassPos struct {
	expandedOffset int
}

func expandSet(s string, isSet2 bool, set1Len int, translateSet2 bool, callCtx *builtins.CallContext, caseClasses *[]caseClassPos, endsWithClass *bool, containsCharClass *bool) ([]byte, error) {
	return expandSetBytes([]byte(s), isSet2, set1Len, translateSet2, callCtx, caseClasses, endsWithClass, containsCharClass, false)
}

func expandSetBytes(data []byte, isSet2 bool, set1Len int, translateSet2 bool, callCtx *builtins.CallContext, caseClasses *[]caseClassPos, endsWithClass *bool, containsCharClass *bool, seenFill bool) ([]byte, error) {
	var result []byte
	lastTokenIsClass := false
	i := 0
	for i < len(data) {
		if len(result) > maxSetLen {
			callCtx.Errf("tr: warning: set expansion exceeded %d bytes, truncating\n", maxSetLen)
			return result[:maxSetLen], nil
		}

		if data[i] == '[' && i+1 < len(data) {
			if i+3 < len(data) && data[i+1] == ':' {
				end := findClosingBracket(data, i+2, ':')
				if end >= 0 {
					className := string(data[i+2 : end])
					if translateSet2 && className != "upper" && className != "lower" {
						return nil, &trError{"when translating, the only character classes that may appear in\nstring2 are 'upper' and 'lower'"}
					}
					if containsCharClass != nil {
						*containsCharClass = true
					}
					chars, err := expandCharClass(className)
					if err != nil {
						return nil, err
					}
					if (className == "upper" || className == "lower") && caseClasses != nil {
						*caseClasses = append(*caseClasses, caseClassPos{expandedOffset: len(result)})
					}
					result = append(result, chars...)
					lastTokenIsClass = true
					i = end + 2
					continue
				}
			}
			if i+3 < len(data) && data[i+1] == '=' {
				end := findClosingBracket(data, i+2, '=')
				if end >= 0 {
					if translateSet2 {
						return nil, &trError{"[=c=] expressions may not appear in string2 when translating"}
					}
					eqChars := data[i+2 : end]
					if len(eqChars) == 0 {
						return nil, &trError{"missing equivalence class character '" + string(data[i:end+2]) + "'"}
					}
					var eqByte byte
					if eqChars[0] == '\\' && len(eqChars) > 1 {
						var adv int
						eqByte, adv, _ = parseBackslashEscapeSingle(eqChars, 0)
						if adv != len(eqChars) {
							return nil, &trError{string(eqChars) + ": equivalence class operand must be a single character"}
						}
					} else if len(eqChars) == 1 {
						eqByte = eqChars[0]
					} else {
						return nil, &trError{string(eqChars) + ": equivalence class operand must be a single character"}
					}
					result = append(result, eqByte)
					lastTokenIsClass = false
					i = end + 2
					continue
				}
			}
			if i+2 < len(data) {
				if rpt, advance, fillCh, isFill := parseRepeat(data, i); advance > 0 {
					if !isSet2 && isFill {
						// GNU tr rejects [c*] (fill) in STRING1, but accepts
						// explicit repeat counts like [c*3] (expands to "ccc").
						return nil, &trError{"the [c*] repeat construct may not appear in string1"}
					}
					if isFill && !translateSet2 {
						// Fill repeats are only meaningful when translating (they
						// pad STRING2 to match STRING1 length).  Explicit counts
						// are accepted in all modes (e.g. -ds squeeze set).
						return nil, &trError{"the [c*] construct may appear in string2 only when translating"}
					}
					if isFill {
						if seenFill {
							return nil, &trError{"only one [c*] repeat construct may appear in string2"}
						}
						var tailClasses []caseClassPos
						var tailEndsWithClass bool
						tail, err := expandSetBytes(data[i+advance:], isSet2, set1Len, translateSet2, callCtx, &tailClasses, &tailEndsWithClass, nil, true)
						if err != nil {
							return nil, err
						}
						needed := max(set1Len-(len(result)+len(tail)), 0)
						needed = min(needed, maxSetLen-len(result))
						for range needed {
							result = append(result, fillCh)
						}
						if caseClasses != nil {
							shift := len(result)
							for _, c := range tailClasses {
								*caseClasses = append(*caseClasses, caseClassPos{expandedOffset: shift + c.expandedOffset})
							}
						}
						result = append(result, tail...)
						lastTokenIsClass = tailEndsWithClass
						if endsWithClass != nil {
							*endsWithClass = lastTokenIsClass
						}
						return result, nil
					}
					result = append(result, rpt...)
					lastTokenIsClass = false
					i += advance
					continue
				} else if advance < 0 {
					return nil, &trError{rptErrMsg(data, i)}
				}
			}
		}

		if data[i] == '\\' && i+1 == len(data) {
			callCtx.Errf("tr: warning: an unescaped backslash at end of string is not portable\n")
			result = append(result, '\\')
			i++
			continue
		}

		var ch byte
		var chAdvance int
		var octalWarn string
		if data[i] == '\\' && i+1 < len(data) {
			ch, chAdvance, octalWarn = parseBackslashEscapeSingle(data, i)
			if octalWarn != "" {
				callCtx.Errf("%s", octalWarn)
			}
		} else {
			ch = data[i]
			chAdvance = 1
		}

		if i+chAdvance < len(data) && data[i+chAdvance] == '-' && i+chAdvance+1 < len(data) {
			rangeEnd := i + chAdvance + 1
			var endCh byte
			var endAdvance int
			if data[rangeEnd] == '\\' && rangeEnd+1 < len(data) {
				endCh, endAdvance, octalWarn = parseBackslashEscapeSingle(data, rangeEnd)
				if octalWarn != "" {
					callCtx.Errf("%s", octalWarn)
				}
			} else {
				endCh = data[rangeEnd]
				endAdvance = 1
			}
			if ch <= endCh {
				for c := ch; ; c++ {
					result = append(result, c)
					if c == endCh {
						break
					}
				}
				lastTokenIsClass = false
				i = rangeEnd + endAdvance
				continue
			}
			return nil, &trError{"range-endpoints of '" + string([]byte{ch}) + "-" + string([]byte{endCh}) + "' are in reverse collating sequence order"}
		}

		result = append(result, ch)
		lastTokenIsClass = false
		i += chAdvance
	}
	if endsWithClass != nil {
		*endsWithClass = lastTokenIsClass
	}
	return result, nil
}

func mapsToSingleByte(set []byte) bool {
	if len(set) <= 1 {
		return true
	}
	first := set[0]
	for _, b := range set[1:] {
		if b != first {
			return false
		}
	}
	return true
}

func findClosingBracket(data []byte, start int, delim byte) int {
	for j := start; j < len(data)-1; j++ {
		if data[j] == delim && data[j+1] == ']' {
			return j
		}
	}
	return -1
}

type charClassDef struct {
	name  string
	chars []byte
}

var charClasses = []charClassDef{
	{"alnum", buildRange('0', '9', 'A', 'Z', 'a', 'z')},
	{"alpha", buildRange('A', 'Z', 'a', 'z')},
	{"blank", []byte{'\t', ' '}},
	{"cntrl", buildCntrl()},
	{"digit", buildRange('0', '9')},
	{"graph", buildRangeInclusive(0x21, 0x7e)},
	{"lower", buildRange('a', 'z')},
	{"print", buildRangeInclusive(0x20, 0x7e)},
	{"punct", buildPunct()},
	{"space", []byte{'\t', '\n', 0x0b, 0x0c, '\r', ' '}},
	{"upper", buildRange('A', 'Z')},
	{"xdigit", buildRange('0', '9', 'A', 'F', 'a', 'f')},
}

func expandCharClass(name string) ([]byte, error) {
	if name == "" {
		return nil, &trError{"missing character class name '[::]'"}
	}
	for _, cc := range charClasses {
		if cc.name == name {
			// Return a copy to prevent callers from mutating the shared
			// package-level charClasses slice.
			return append([]byte(nil), cc.chars...), nil
		}
	}
	return nil, &trError{"invalid character class '" + name + "'"}
}

// buildRange returns all bytes in the given inclusive ranges.
// pairs must have even length: [start1, end1, start2, end2, ...].
func buildRange(pairs ...byte) []byte {
	if len(pairs)%2 != 0 {
		panic("buildRange: pairs must have even length")
	}
	var result []byte
	for i := 0; i < len(pairs); i += 2 {
		for c := pairs[i]; ; c++ {
			result = append(result, c)
			if c == pairs[i+1] {
				break
			}
		}
	}
	return result
}

func buildRangeInclusive(start, end byte) []byte {
	result := make([]byte, 0, int(end)-int(start)+1)
	for c := start; ; c++ {
		result = append(result, c)
		if c == end {
			break
		}
	}
	return result
}

func buildCntrl() []byte {
	var result []byte
	for c := byte(0); c <= 0x1f; c++ {
		result = append(result, c)
	}
	result = append(result, 0x7f)
	return result
}

func buildPunct() []byte {
	var result []byte
	for c := byte(0x21); c <= byte(0x7e); c++ {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			continue
		}
		result = append(result, c)
	}
	return result
}

func parseBackslashEscapeSingle(data []byte, pos int) (byte, int, string) {
	if pos+1 >= len(data) {
		return '\\', 1, ""
	}
	next := data[pos+1]
	switch next {
	case 'a':
		return '\a', 2, ""
	case 'b':
		return '\b', 2, ""
	case 'f':
		return '\f', 2, ""
	case 'n':
		return '\n', 2, ""
	case 'r':
		return '\r', 2, ""
	case 't':
		return '\t', 2, ""
	case 'v':
		return '\v', 2, ""
	case '\\':
		return '\\', 2, ""
	}
	if next >= '0' && next <= '7' {
		return parseOctal(data, pos+1)
	}
	return next, 2, ""
}

func parseOctal(data []byte, start int) (byte, int, string) {
	val := 0
	count := 0
	for i := start; i < len(data) && count < 3; i++ {
		if data[i] < '0' || data[i] > '7' {
			break
		}
		val = val*8 + int(data[i]-'0')
		count++
	}
	var warning string
	if val > 255 {
		origEscape := string(data[start : start+count])
		val = val / 8
		count--
		resultEscape := "\\0" + string(data[start:start+count])
		// Safe: the loop consumed exactly 3 digits within data, so after
		// decrementing count to 2, start+count+1 == start+3 <= len(data).
		// Defensive check guards against future refactors breaking this invariant.
		if start+count >= len(data) {
			return byte(val), count + 1, ""
		}
		trailingChar := string(data[start+count : start+count+1])
		warning = fmt.Sprintf("tr: warning: the ambiguous octal escape \\%s is being\n\tinterpreted as the 2-byte sequence %s, %s\n", origEscape, resultEscape, trailingChar)
	}
	return byte(val), count + 1, warning
}

func parseRepeat(data []byte, pos int) ([]byte, int, byte, bool) {
	if pos+2 >= len(data) || data[pos] != '[' {
		return nil, 0, 0, false
	}

	ch := data[pos+1]
	charAdvance := 1
	if ch == '\\' && pos+3 < len(data) {
		var adv int
		ch, adv, _ = parseBackslashEscapeSingle(data, pos+1)
		charAdvance = adv
	}

	starIdx := pos + 1 + charAdvance

	if starIdx >= len(data) || data[starIdx] != '*' {
		return nil, 0, 0, false
	}

	closeIdx := -1
	for j := starIdx + 1; j < len(data); j++ {
		if data[j] == ']' {
			closeIdx = j
			break
		}
	}
	if closeIdx < 0 {
		return nil, 0, 0, false
	}

	countStr := string(data[starIdx+1 : closeIdx])
	advance := closeIdx - pos + 1

	if countStr == "" {
		return nil, advance, ch, true
	}

	// Reject negative repeat counts (e.g. [b*-0], [b*-1]).
	// strconv.ParseInt("-0", 10, 64) returns 0, which would
	// incorrectly be treated as fill mode.  GNU tr rejects these.
	if countStr[0] == '-' {
		return nil, -advance, 0, false
	}

	var count int64
	// GNU-compatible heuristic: "0" alone is decimal (means fill), but a
	// leading zero with additional digits (e.g. "010") is octal.  This
	// matches GNU coreutils tr behaviour where [c*0] means fill and
	// [c*010] is octal 8.
	if len(countStr) > 1 && countStr[0] == '0' {
		var parseErr error
		count, parseErr = strconv.ParseInt(countStr, 8, 64)
		if parseErr != nil {
			return nil, -advance, 0, false
		}
	} else {
		var parseErr error
		count, parseErr = strconv.ParseInt(countStr, 10, 64)
		if parseErr != nil {
			return nil, -advance, 0, false
		}
	}

	if count == 0 {
		return nil, advance, ch, true
	}
	if count < 0 {
		return nil, -advance, 0, false
	}

	const maxRepeat = 1 << 20
	count = min(count, maxRepeat)

	result := make([]byte, count)
	for i := range result {
		result[i] = ch
	}
	return result, advance, 0, false
}

func rptErrMsg(data []byte, pos int) string {
	if pos+2 >= len(data) {
		return "invalid repeat construct"
	}
	charAdvance := 1
	if data[pos+1] == '\\' && pos+3 < len(data) {
		_, charAdvance, _ = parseBackslashEscapeSingle(data, pos+1)
	}
	starIdx := pos + 1 + charAdvance
	closeIdx := -1
	for j := starIdx + 1; j < len(data); j++ {
		if data[j] == ']' {
			closeIdx = j
			break
		}
	}
	if closeIdx < 0 {
		return "invalid repeat construct"
	}
	countStr := string(data[starIdx+1 : closeIdx])
	if countStr == "" {
		return "invalid repeat construct"
	}
	return "invalid repeat count '" + countStr + "' in [c*n] construct"
}

type trError struct {
	msg string
}

func (e *trError) Error() string {
	return e.msg
}
