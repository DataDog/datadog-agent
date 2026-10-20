// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find

import (
	iofs "io/fs"
	"math"
	"strings"
	"unicode/utf8"
)

// matchPerm checks whether filePerm matches the target permission bits
// according to the comparison mode:
//
//	'=' exact: filePerm == target
//	'-' all bits: filePerm & target == target
//	'/' any bit: filePerm & target != 0 (special: if target==0, always true)
func matchPerm(filePerm iofs.FileMode, target uint32, cmpMode byte) bool {
	// Go stores setuid/setgid/sticky as high flag bits (ModeSetuid, etc.),
	// not in the Unix 12-bit position. Convert to Unix-style 12-bit mode.
	fp := uint32(filePerm.Perm()) // bits 8-0 (rwxrwxrwx)
	if filePerm&iofs.ModeSetuid != 0 {
		fp |= 0o4000
	}
	if filePerm&iofs.ModeSetgid != 0 {
		fp |= 0o2000
	}
	if filePerm&iofs.ModeSticky != 0 {
		fp |= 0o1000
	}
	switch cmpMode {
	case '-':
		return fp&target == target
	case '/':
		if target == 0 {
			return true // GNU find: -perm /0 matches everything
		}
		return fp&target != 0
	default: // '='
		return fp == target
	}
}

// matchGlob matches a name against a glob pattern.
// Uses pathGlobMatch which correctly handles [!...] negated character classes
// and treats malformed brackets (e.g. unclosed '[') as literal characters (or non-matching for incomplete ranges),
// matching GNU find's fnmatch() behaviour.
func matchGlob(pattern, name string) bool {
	return pathGlobMatch(pattern, name)
}

// matchGlobFold matches a name against a glob pattern case-insensitively.
func matchGlobFold(pattern, name string) bool {
	return pathGlobMatch(strings.ToLower(pattern), strings.ToLower(name))
}

// matchType checks if a file's type matches the -type argument.
// typeArg may contain comma-separated types (GNU extension).
func matchType(info iofs.FileInfo, typeArg string) bool {
	fileType := fileTypeChar(info)

	// Handle comma-separated types.
	for _, c := range typeArg {
		if c != ',' && byte(c) == fileType {
			return true
		}
	}
	return false
}

// fileTypeChar returns the find type character for a file's mode.
// Accepts FileInfo (not FileMode) to avoid adding io/fs.FileMode to the
// import allowlist — matches the pattern used by ls.go.
func fileTypeChar(info iofs.FileInfo) byte {
	mode := info.Mode()
	switch {
	case mode.IsRegular():
		return 'f'
	case mode&iofs.ModeDir != 0:
		return 'd'
	case mode&iofs.ModeSymlink != 0:
		return 'l'
	case mode&iofs.ModeNamedPipe != 0:
		return 'p'
	case mode&iofs.ModeSocket != 0:
		return 's'
	case mode&iofs.ModeCharDevice != 0:
		return 'c'
	case mode&iofs.ModeDevice != 0:
		return 'b'
	default:
		return '?'
	}
}

// sizeBlockSize returns the block size for rounding up in exact comparisons.
func sizeBlockSize(unit byte) int64 {
	switch unit {
	case 'c':
		return 1
	case 'w':
		return 2
	case 'b':
		return 512
	case 'k':
		return 1024
	case 'M':
		return 1024 * 1024
	case 'G':
		return 1024 * 1024 * 1024
	default:
		return 512
	}
}

// compareSize checks if fileSize matches the size predicate.
// GNU find rounds up to units for exact match: a 1-byte file is +0c, 1c, -2c.
func compareSize(fileSize int64, su sizeUnit) bool {
	blockSz := sizeBlockSize(su.unit)
	// Round file size up to the next block (ceiling division).
	// Guard against overflow: (fileSize + blockSz - 1) can exceed MaxInt64
	// when fileSize is close to MaxInt64.
	var fileBlocks int64
	if fileSize > 0 {
		if blockSz == 1 {
			fileBlocks = fileSize
		} else if fileSize <= math.MaxInt64-blockSz+1 {
			fileBlocks = (fileSize + blockSz - 1) / blockSz
		} else {
			// Overflow-safe ceiling division for very large file sizes.
			fileBlocks = fileSize/blockSz + 1
		}
	}

	switch su.cmp {
	case cmpMore: // +n: strictly greater than n units
		return fileBlocks > su.n
	case cmpLess: // -n: strictly less than n units
		return fileBlocks < su.n
	default: // exactly n units
		return fileBlocks == su.n
	}
}

// compareNumeric compares a value with the cmp operator.
func compareNumeric(actual, target int64, cmp cmpOp) bool {
	switch cmp {
	case cmpMore: // +n: strictly greater
		return actual > target
	case cmpLess: // -n: strictly less
		return actual < target
	default: // exactly n
		return actual == target
	}
}

// baseName returns the last element of a path.
// Trailing slashes are stripped first so that "dir/" returns "dir",
// matching GNU find's behavior for -name/-iname matching.
// Only '/' is checked because the find builtin normalizes all input
// paths and predicate arguments to forward slashes via filepath.ToSlash
// at parse time, and joinPath produces '/'-separated child paths.
func baseName(p string) string {
	// Strip trailing slashes (but keep at least one char for root "/").
	for len(p) > 1 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			tail := p[i+1:]
			if len(tail) == 0 {
				// Root path "/" — return "/" as the basename.
				return "/"
			}
			return tail
		}
	}
	return p
}

// matchPathGlob matches a full path against a glob pattern where '*' crosses
// '/' (FNM_PATHNAME-free). This matches GNU find's -path behaviour.
func matchPathGlob(pattern, name string) bool {
	return pathGlobMatch(pattern, name)
}

// matchPathGlobFold is like matchPathGlob but case-insensitive.
func matchPathGlobFold(pattern, name string) bool {
	return pathGlobMatch(strings.ToLower(pattern), strings.ToLower(name))
}

// pathGlobMatch implements glob matching where '*' matches any character
// including '/', '?' matches exactly one rune including '/', and
// '[...]' character classes match runes as in path.Match.
func pathGlobMatch(pattern, name string) bool {
	px, nx := 0, 0
	// nextPx/nextNx track the position to retry when a '*' fails to match.
	nextPx, nextNx := 0, 0
	starActive := false

	for px < len(pattern) || nx < len(name) {
		if px < len(pattern) {
			switch pattern[px] {
			case '*':
				// '*' matches zero or more of any character (including '/').
				// Record restart point and try matching zero chars first.
				starActive = true
				nextPx = px
				nextNx = nx + 1
				px++
				continue
			case '?':
				// '?' matches exactly one rune (including '/').
				if nx < len(name) {
					_, w := utf8.DecodeRuneInString(name[nx:])
					px++
					nx += w
					continue
				}
			case '[':
				// Character class — delegate to matchClass for the class portion.
				if nx < len(name) {
					r, w := utf8.DecodeRuneInString(name[nx:])
					matched, patWidth := matchClass(pattern[px:], r)
					if matched {
						px += patWidth
						nx += w
						continue
					}
					// Malformed class (patWidth==0): fall back to literal or fail.
					if patWidth == 0 && pattern[px] == name[nx] {
						px++
						nx++
						continue
					}
					// Fatally malformed (patWidth==-1): pattern cannot match.
					if patWidth == -1 {
						return false
					}
				}
			case '\\':
				// Escape: next character is literal.
				px++
				if px >= len(pattern) {
					// Trailing backslash with no character to escape
					// (dangling escape). GNU find's fnmatch treats this
					// as non-matching, so fall through to backtrack/fail.
				} else if nx < len(name) && pattern[px] == name[nx] {
					px++
					nx++
					continue
				}
			default:
				if nx < len(name) && pattern[px] == name[nx] {
					px++
					nx++
					continue
				}
			}
		}
		// Current characters don't match. Backtrack to last '*' if possible.
		if starActive && nextNx <= len(name) {
			px = nextPx + 1
			nx = nextNx
			nextNx++
			continue
		}
		return false
	}
	return true
}

// matchClass tries to match a single rune against a bracket expression
// starting at pattern[0] == '['. Returns (matched, width) where width is
// the number of bytes consumed from pattern (including the closing ']').
// On malformed classes returns (false, 0) for benign unclosed brackets
// (caller falls back to literal '[') or (false, -1) for incomplete ranges
// like "[a-" where the dash has no following character (caller treats as
// non-matching, per GNU fnmatch behavior).
func matchClass(pattern string, ch rune) (bool, int) {
	if len(pattern) < 2 || pattern[0] != '[' {
		return false, 0
	}
	i := 1
	negate := false
	if i < len(pattern) && pattern[i] == '^' {
		negate = true
		i++
	} else if i < len(pattern) && pattern[i] == '!' {
		negate = true
		i++
	}
	matched := false
	first := true
	for i < len(pattern) {
		if pattern[i] == ']' && !first {
			i++ // consume ']'
			if negate {
				return !matched, i
			}
			return matched, i
		}
		first = false
		// Handle backslash escaping inside bracket classes:
		// \] matches literal ], \\ matches literal \, etc.
		lo, loW := utf8.DecodeRuneInString(pattern[i:])
		if lo == '\\' && i+loW < len(pattern) {
			lo, loW = utf8.DecodeRuneInString(pattern[i+loW:])
			i++ // skip the 1-byte backslash
		}
		i += loW
		hi := lo
		if i+1 < len(pattern) && pattern[i] == '-' && pattern[i+1] != ']' {
			var hiW int
			hi, hiW = utf8.DecodeRuneInString(pattern[i+1:])
			if hi == '\\' && i+1+hiW < len(pattern) {
				hi, hiW = utf8.DecodeRuneInString(pattern[i+1+hiW:])
				i++ // skip the 1-byte backslash
			}
			i += 1 + hiW
		} else if i < len(pattern) && pattern[i] == '-' && i+1 >= len(pattern) {
			// Incomplete range: dash at end of pattern with no range-end
			// character. GNU fnmatch treats this as non-matching rather
			// than falling back to literal '['.
			return false, -1
		}
		if lo <= ch && ch <= hi {
			matched = true
		}
	}
	// Unclosed bracket — malformed.
	return false, 0
}
