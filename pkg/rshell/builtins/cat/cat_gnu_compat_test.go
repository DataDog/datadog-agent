// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCatDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	}
	return dir
}

func setupCatDirBytes(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0644))
	}
	return dir
}

// TestGNUCompatCatPlain — plain cat outputs file contents verbatim.
//
// GNU command: gcat five.txt
// Expected:    "alpha\nbeta\ngamma\ndelta\nepsilon\n"
func TestGNUCompatCatPlain(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"five.txt": fiveLines})
	stdout, _, code := cmdRun(t, "cat five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

// TestGNUCompatCatNumberN — -n numbers all lines.
//
// GNU command: gcat -n three.txt (three.txt = "a\nb\nc\n")
// Expected:    "     1\ta\n     2\tb\n     3\tc\n"
func TestGNUCompatCatNumberN(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"three.txt": "a\nb\nc\n"})
	stdout, _, code := cmdRun(t, "cat -n three.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n     2\tb\n     3\tc\n", stdout)
}

// TestGNUCompatCatNumberNWithBlanks — -n numbers blank lines too.
//
// GNU command: gcat -n blanks.txt (blanks.txt = "a\n\nb\n")
// Expected:    "     1\ta\n     2\t\n     3\tb\n"
func TestGNUCompatCatNumberNWithBlanks(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"blanks.txt": "a\n\nb\n"})
	stdout, _, code := cmdRun(t, "cat -n blanks.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n     2\t\n     3\tb\n", stdout)
}

// TestGNUCompatCatNumberNonblankB — -b skips blank lines.
//
// GNU command: gcat -b blanks.txt (blanks.txt = "a\n\nb\n")
// Expected:    "     1\ta\n\n     2\tb\n"
func TestGNUCompatCatNumberNonblankB(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"blanks.txt": "a\n\nb\n"})
	stdout, _, code := cmdRun(t, "cat -b blanks.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n\n     2\tb\n", stdout)
}

// TestGNUCompatCatSqueezeBlankS — -s squeezes consecutive blank lines.
//
// GNU command: gcat -s squeeze.txt (squeeze.txt = "a\n\n\n\nb\n")
// Expected:    "a\n\nb\n"
func TestGNUCompatCatSqueezeBlankS(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"squeeze.txt": "a\n\n\n\nb\n"})
	stdout, _, code := cmdRun(t, "cat -s squeeze.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n\nb\n", stdout)
}

// TestGNUCompatCatShowEndsE — -E displays $ at end of each line.
//
// GNU command: gcat -E ends.txt (ends.txt = "alpha\nbeta\n")
// Expected:    "alpha$\nbeta$\n"
func TestGNUCompatCatShowEndsE(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"ends.txt": "alpha\nbeta\n"})
	stdout, _, code := cmdRun(t, "cat -E ends.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha$\nbeta$\n", stdout)
}

// TestGNUCompatCatShowEndsCRLF — -E converts \r before \n to ^M.
//
// GNU command: printf 'a\r\nb\n' | gcat -E
// Expected:    "a^M$\nb$\n"
func TestGNUCompatCatShowEndsCRLF(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"crlf.txt": "a\r\nb\n"})
	stdout, _, code := cmdRun(t, "cat -E crlf.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a^M$\nb$\n", stdout)
}

// TestGNUCompatCatShowTabsT — -T displays TAB as ^I.
//
// GNU command: gcat -T tabs.txt (tabs.txt = "a\tb\n")
// Expected:    "a^Ib\n"
func TestGNUCompatCatShowTabsT(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"tabs.txt": "a\tb\n"})
	stdout, _, code := cmdRun(t, "cat -T tabs.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a^Ib\n", stdout)
}

// TestGNUCompatCatShowNonprintingV — -v shows control chars.
//
// GNU command: printf '\x00\x01\x1f\n' | gcat -v
// Expected:    "^@^A^_\n"
func TestGNUCompatCatShowNonprintingV(t *testing.T) {
	dir := setupCatDirBytes(t, map[string][]byte{"ctrl.bin": {0x00, 0x01, 0x1f, '\n'}})
	stdout, _, code := cmdRun(t, "cat -v ctrl.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@^A^_\n", stdout)
}

// TestGNUCompatCatShowNonprintingHighBytes — -v shows M- notation for high bytes.
//
// GNU command: printf '\x80\x9f\xa0\xfe\xff\n' | gcat -v
// Expected:    "M-^@M-^_M- M-~M-^?\n"
func TestGNUCompatCatShowNonprintingHighBytes(t *testing.T) {
	dir := setupCatDirBytes(t, map[string][]byte{"high.bin": {0x80, 0x9f, 0xa0, 0xfe, 0xff, '\n'}})
	stdout, _, code := cmdRun(t, "cat -v high.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "M-^@M-^_M- M-~M-^?\n", stdout)
}

// TestGNUCompatCatShowAllA — -A is equivalent to -vET.
//
// GNU command: printf '\x00\ta\n' | gcat -A
// Expected:    "^@^Ia$\n"
func TestGNUCompatCatShowAllA(t *testing.T) {
	dir := setupCatDirBytes(t, map[string][]byte{"all.bin": {0x00, '\t', 'a', '\n'}})
	stdout, _, code := cmdRun(t, "cat -A all.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@^Ia$\n", stdout)
}

// TestGNUCompatCatEmptyFile — empty file produces no output.
//
// GNU command: gcat empty.txt (empty.txt is 0 bytes)
// Expected:    ""
func TestGNUCompatCatEmptyFile(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"empty.txt": ""})
	stdout, _, code := cmdRun(t, "cat empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatCatNoTrailingNewline — file without trailing newline.
//
// GNU command: printf 'no newline' | gcat
// Expected:    "no newline"
func TestGNUCompatCatNoTrailingNewline(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"noterm.txt": "no newline"})
	stdout, _, code := cmdRun(t, "cat noterm.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "no newline", stdout)
}

// TestGNUCompatCatShowEndsNoTrailingNewline — -E with no trailing newline.
//
// GNU command: printf 'hello' | gcat -E
// Expected:    "hello" (no $ because no newline)
func TestGNUCompatCatShowEndsNoTrailingNewline(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"noterm.txt": "hello"})
	stdout, _, code := cmdRun(t, "cat -E noterm.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

// TestGNUCompatCatVPreservesTab — -v does not convert TAB.
//
// GNU command: printf 'a\tb\n' | gcat -v
// Expected:    "a\tb\n"
func TestGNUCompatCatVPreservesTab(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"tabs.txt": "a\tb\n"})
	stdout, _, code := cmdRun(t, "cat -v tabs.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\tb\n", stdout)
}

// TestGNUCompatCatBOverridesN — -b overrides -n regardless of order.
//
// GNU command: gcat -n -b blanks.txt
// Expected:    "     1\ta\n\n     2\tb\n"
func TestGNUCompatCatBOverridesN(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"blanks.txt": "a\n\nb\n"})
	stdout, _, code := cmdRun(t, "cat -n -b blanks.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n\n     2\tb\n", stdout)
}

// TestGNUCompatCatFlagEComposite — -e enables -v and -E.
//
// GNU command: printf '\x00a\n' | gcat -e
// Expected:    "^@a$\n"
func TestGNUCompatCatFlagEComposite(t *testing.T) {
	dir := setupCatDirBytes(t, map[string][]byte{"file.bin": {0x00, 'a', '\n'}})
	stdout, _, code := cmdRun(t, "cat -e file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@a$\n", stdout)
}

// TestGNUCompatCatFlagTComposite — -t enables -v and -T.
//
// GNU command: printf '\x00\t\n' | gcat -t
// Expected:    "^@^I\n"
func TestGNUCompatCatFlagTComposite(t *testing.T) {
	dir := setupCatDirBytes(t, map[string][]byte{"file.bin": {0x00, '\t', '\n'}})
	stdout, _, code := cmdRun(t, "cat -t file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@^I\n", stdout)
}

// TestGNUCompatCatNumberAcrossFiles — line numbers continue across files.
//
// GNU command: gcat -n a.txt b.txt  (a.txt="one\n", b.txt="two\nthree\n")
// Expected:    "     1\tone\n     2\ttwo\n     3\tthree\n"
func TestGNUCompatCatNumberAcrossFiles(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"a.txt": "one\n", "b.txt": "two\nthree\n"})
	stdout, _, code := cmdRun(t, "cat -n a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\tone\n     2\ttwo\n     3\tthree\n", stdout)
}

// TestGNUCompatCatSqueezeWithNumber — -sn squeezes before numbering.
//
// GNU command: gcat -sn squeeze.txt (squeeze.txt = "a\n\n\nb\n")
// Expected:    "     1\ta\n     2\t\n     3\tb\n"
func TestGNUCompatCatSqueezeWithNumber(t *testing.T) {
	dir := setupCatDir(t, map[string]string{"squeeze.txt": "a\n\n\nb\n"})
	stdout, _, code := cmdRun(t, "cat -sn squeeze.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n     2\t\n     3\tb\n", stdout)
}
