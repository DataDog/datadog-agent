// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find

import (
	iofs "io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathGlobMatchTrailingBackslash(t *testing.T) {
	// A trailing backslash is a dangling escape (no character to escape).
	// GNU find's fnmatch treats this as non-matching for any input,
	// including a literal backslash character.
	assert.False(t, pathGlobMatch(`abc\`, `abc\`))
	assert.False(t, pathGlobMatch(`abc\`, `abcd`))
	assert.False(t, pathGlobMatch(`abc\`, `abc`))
	assert.False(t, pathGlobMatch(`\`, `\`))
	assert.False(t, pathGlobMatch(`*\`, `abc\`))

	// Properly escaped backslash (\\) DOES match a literal backslash.
	assert.True(t, pathGlobMatch(`abc\\`, `abc\`))
	assert.True(t, pathGlobMatch(`\\`, `\`))
	assert.True(t, pathGlobMatch(`*\\`, `abc\`))
}

func TestMatchGlobMalformedBracket(t *testing.T) {
	// Unclosed bracket patterns fall back to literal comparison.
	assert.True(t, matchGlob("[", "["))
	assert.False(t, matchGlob("[", "a"))
	assert.True(t, matchGlob("[abc", "[abc"))
	assert.False(t, matchGlob("[abc", "a"))

	// Incomplete range (trailing dash) — non-matching per GNU fnmatch.
	assert.False(t, matchGlob("[a-", "[a-"))
	assert.False(t, matchGlob("[a-", "a"))
	assert.False(t, matchGlob("[ab-", "[ab-"))
}

func TestMatchGlobFoldMalformedBracket(t *testing.T) {
	assert.True(t, matchGlobFold("[", "["))
	assert.False(t, matchGlobFold("[", "a"))

	// Incomplete range — non-matching.
	assert.False(t, matchGlobFold("[a-", "[a-"))
}

func TestBaseNameEdgeCases(t *testing.T) {
	assert.Equal(t, "dir", baseName("dir"))
	assert.Equal(t, "dir", baseName("dir/"))
	assert.Equal(t, "dir", baseName("dir//"))
	assert.Equal(t, "dir", baseName("/path/to/dir"))
	assert.Equal(t, "dir", baseName("/path/to/dir/"))
	assert.Equal(t, "/", baseName("/"))
	assert.Equal(t, "/", baseName("///"))
	assert.Equal(t, "file", baseName("file"))
	assert.Equal(t, ".", baseName("."))
	assert.Equal(t, ".", baseName("./"))
	assert.Equal(t, "b", baseName("a/b"))
	assert.Equal(t, "b", baseName("a/b/"))
}

func TestMatchClassEdgeCases(t *testing.T) {
	// Valid class
	matched, width := matchClass("[abc]", 'a')
	assert.True(t, matched)
	assert.Equal(t, 5, width)

	// Non-matching valid class
	matched, width = matchClass("[abc]", 'z')
	assert.False(t, matched)
	assert.Equal(t, 5, width)

	// Negated class
	matched, width = matchClass("[!abc]", 'z')
	assert.True(t, matched)
	assert.Equal(t, 6, width)

	matched, width = matchClass("[^abc]", 'a')
	assert.False(t, matched)
	assert.Equal(t, 6, width)

	// Range
	matched, width = matchClass("[a-z]", 'm')
	assert.True(t, matched)
	assert.Equal(t, 5, width)

	matched, width = matchClass("[a-z]", 'A')
	assert.False(t, matched)
	assert.Equal(t, 5, width)

	// Malformed (unclosed)
	matched, width = matchClass("[abc", 'a')
	assert.False(t, matched)
	assert.Equal(t, 0, width)

	// Single char "[" — too short
	matched, width = matchClass("[", 'a')
	assert.False(t, matched)
	assert.Equal(t, 0, width)

	// "]" as first char in class (literal, not closing)
	matched, width = matchClass("[]abc]", ']')
	assert.True(t, matched)
	assert.Equal(t, 6, width)

	// Backslash escape inside class: [\]] matches literal ]
	matched, width = matchClass("[\\]]", ']')
	assert.True(t, matched)
	assert.Equal(t, 4, width)

	matched, width = matchClass("[\\]]", 'a')
	assert.False(t, matched)
	assert.Equal(t, 4, width)

	// Backslash escape: [a\]] matches a or ]
	matched, width = matchClass("[a\\]]", ']')
	assert.True(t, matched)
	assert.Equal(t, 5, width)

	matched, width = matchClass("[a\\]]", 'a')
	assert.True(t, matched)
	assert.Equal(t, 5, width)

	// Backslash escape: [\\a] matches \ or a
	matched, width = matchClass("[\\\\a]", '\\')
	assert.True(t, matched)
	assert.Equal(t, 5, width)

	matched, width = matchClass("[\\\\a]", 'a')
	assert.True(t, matched)
	assert.Equal(t, 5, width)

	matched, width = matchClass("[\\\\a]", 'z')
	assert.False(t, matched)
	assert.Equal(t, 5, width)

	// Escaped multi-byte character inside class: [\é] matches é
	matched, width = matchClass(`[\é]`, 'é')
	assert.True(t, matched)
	assert.Equal(t, 5, width) // [ + \ + é(2 bytes) + ] = 5

	matched, width = matchClass(`[\é]`, 'a')
	assert.False(t, matched)
	assert.Equal(t, 5, width)

	// Escaped multi-byte range endpoints: [\é-\ü]
	matched, width = matchClass(`[\é-\ü]`, 'ö') // ö is between é and ü
	assert.True(t, matched)

	matched, _ = matchClass(`[\é-\ü]`, 'a')
	assert.False(t, matched)
}

// TestFileTypeChar verifies fileTypeChar returns the correct character for each file mode.
func TestFileTypeChar(t *testing.T) {
	tests := []struct {
		name string
		mode iofs.FileMode
		want byte
	}{
		{"regular file", 0o644, 'f'},
		{"directory", iofs.ModeDir, 'd'},
		{"symlink", iofs.ModeSymlink, 'l'},
		{"named pipe", iofs.ModeNamedPipe, 'p'},
		{"socket", iofs.ModeSocket, 's'},
		{"char device", iofs.ModeCharDevice, 'c'},
		{"block device", iofs.ModeDevice, 'b'},
		{"both device bits set", iofs.ModeDevice | iofs.ModeCharDevice, 'c'},
		{"irregular", iofs.ModeIrregular, '?'},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &fakeFileInfo{mode: tt.mode}
			got := fileTypeChar(info)
			assert.Equal(t, tt.want, got, "fileTypeChar(mode=%v)", tt.mode)
		})
	}
}

// TestMatchType verifies matchType with block and char device types.
func TestMatchType(t *testing.T) {
	blockDev := &fakeFileInfo{mode: iofs.ModeDevice}
	charDev := &fakeFileInfo{mode: iofs.ModeCharDevice}
	regular := &fakeFileInfo{mode: 0o644}

	tests := []struct {
		name    string
		info    iofs.FileInfo
		typeArg string
		want    bool
	}{
		{"block matches -type b", blockDev, "b", true},
		{"block no match -type c", blockDev, "c", false},
		{"char matches -type c", charDev, "c", true},
		{"char no match -type b", charDev, "b", false},
		{"block matches -type b,c", blockDev, "b,c", true},
		{"char matches -type b,c", charDev, "b,c", true},
		{"regular no match -type b", regular, "b", false},
		{"regular no match -type c", regular, "c", false},
		{"regular no match -type b,c", regular, "b,c", false},
		{"regular matches -type f", regular, "f", true},
		{"block no match -type f", blockDev, "f", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchType(tt.info, tt.typeArg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareNumeric(t *testing.T) {
	// Exact match
	assert.True(t, compareNumeric(5, 5, cmpExact))
	assert.False(t, compareNumeric(5, 6, cmpExact))

	// Greater than
	assert.True(t, compareNumeric(6, 5, cmpMore))
	assert.False(t, compareNumeric(5, 5, cmpMore))
	assert.False(t, compareNumeric(4, 5, cmpMore))

	// Less than
	assert.True(t, compareNumeric(4, 5, cmpLess))
	assert.False(t, compareNumeric(5, 5, cmpLess))
	assert.False(t, compareNumeric(6, 5, cmpLess))
}

func TestMatchPerm(t *testing.T) {
	tests := []struct {
		name     string
		filePerm iofs.FileMode
		target   uint32
		cmpMode  byte
		want     bool
	}{
		// Exact match
		{"exact 644 match", 0o644, 0o644, '=', true},
		{"exact 644 no match 755", 0o755, 0o644, '=', false},
		{"exact 0 match", 0, 0, '=', true},
		{"exact 777 match", 0o777, 0o777, '=', true},

		// All bits set (-)
		{"all bits 0111 on 755", 0o755, 0o111, '-', true},
		{"all bits 0111 on 644", 0o644, 0o111, '-', false},
		{"all bits 0444 on 644", 0o644, 0o444, '-', true},
		{"all bits 0 always true", 0o644, 0, '-', true},
		{"all bits 0777 on 755", 0o755, 0o777, '-', false},
		{"all bits 0777 on 777", 0o777, 0o777, '-', true},

		// Any bit set (/)
		{"any bit 0111 on 755", 0o755, 0o111, '/', true},
		{"any bit 0111 on 644", 0o644, 0o111, '/', false},
		{"any bit 0222 on 644", 0o644, 0o222, '/', true},
		{"any bit 0 always true", 0o644, 0, '/', true},

		// Special bits (setuid/setgid/sticky) — Go uses high flag bits
		{"setuid exact", 0o755 | iofs.ModeSetuid, 0o4755, '=', true},
		{"setuid all bits", 0o755 | iofs.ModeSetuid, 0o4000, '-', true},
		{"setuid not set", 0o755, 0o4000, '-', false},
		{"setgid any bit", 0o755 | iofs.ModeSetgid, 0o2000, '/', true},
		{"sticky exact", 0o755 | iofs.ModeSticky, 0o1755, '=', true},
		{"any bit 0001 on 644", 0o644, 0o001, '/', false},
		{"any bit 0100 on 755", 0o755, 0o100, '/', true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPerm(tt.filePerm, tt.target, tt.cmpMode)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPathGlobMatchMalformedBracket(t *testing.T) {
	// Unclosed bracket patterns fall back to literal comparison.
	assert.True(t, pathGlobMatch("[", "["))
	assert.False(t, pathGlobMatch("[", "a"))
	assert.True(t, pathGlobMatch("dir/[sub/file", "dir/[sub/file"))
	assert.False(t, pathGlobMatch("dir/[sub/file", "dir/asub/file"))
	// Star followed by malformed bracket (backtracking interaction).
	assert.True(t, pathGlobMatch("*/[", "dir/["))
	assert.False(t, pathGlobMatch("*/[", "dir/a"))

	// Incomplete range (trailing dash) — non-matching per GNU fnmatch.
	assert.False(t, pathGlobMatch("[a-", "[a-"))
	assert.False(t, pathGlobMatch("dir/[a-", "dir/[a-"))

	// Escaped multi-byte character in bracket class.
	assert.True(t, pathGlobMatch(`[\é]`, "é"))
	assert.False(t, pathGlobMatch(`[\é]`, "a"))
}
