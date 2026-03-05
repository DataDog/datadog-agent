// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

// GNU equivalence tests for the grep builtin.
//
// Reference outputs were captured from GNU grep 3.11 (Homebrew):
//   /opt/homebrew/bin/ggrep
//
// These tests assert byte-for-byte output equivalence between our builtin and
// GNU grep for the cases most sensitive to formatting (line numbers, filenames,
// context separators, count mode, only-matching, exit codes).

package grep_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// gnuCompatGrepRun creates named files in a temp dir and runs a grep script.
func gnuCompatGrepRun(t *testing.T, files map[string]string, script string) (string, string, int) {
	t.Helper()
	dir := setupGrepDir(t, files)
	return grepRun(t, script, dir)
}

// ── Line number flag ──────────────────────────────────────────────────────────

// TestGNUCompatGrepLineNumber — -n prefixes each matched line with its 1-based number.
//
// GNU command: ggrep -n . three.txt   (three.txt = "alpha\nbeta\ngamma\n")
// Expected:    "1:alpha\n2:beta\n3:gamma\n"
func TestGNUCompatGrepLineNumber(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"grep -n . three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "1:alpha\n2:beta\n3:gamma\n", stdout)
}

// ── Count mode ────────────────────────────────────────────────────────────────

// TestGNUCompatGrepCount — -c prints the count of matching lines.
//
// GNU command: ggrep -c match match.txt   (match.txt = "match\nnomatch\nmatch\n")
// Expected:    "3\n"  (all three lines contain "match")
func TestGNUCompatGrepCount(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"match.txt": "match\nnomatch\nmatch\n"},
		"grep -c match match.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "3\n", stdout)
}

// TestGNUCompatGrepCountZero — -c returns 0 when no lines match.
//
// GNU command: ggrep -c ZZNOTFOUND match.txt
// Expected:    "0\n"
func TestGNUCompatGrepCountZero(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"match.txt": "match\nnomatch\nmatch\n"},
		"grep -c ZZNOTFOUND match.txt",
	)
	assert.Equal(t, 1, exitCode) // no matches → exit 1
	assert.Equal(t, "0\n", stdout)
}

// ── Files-with-matches / files-without-match ──────────────────────────────────

// TestGNUCompatGrepFilesWithMatches — -l prints only names of files containing matches.
//
// GNU command: ggrep -l match match.txt one.txt
//   (match.txt contains "match"; one.txt = "only one line\n")
// Expected:    "match.txt\n"
func TestGNUCompatGrepFilesWithMatches(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{
			"match.txt": "match\nnomatch\nmatch\n",
			"one.txt":   "only one line\n",
		},
		"grep -l match match.txt one.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "match.txt\n", stdout)
}

// TestGNUCompatGrepFilesWithoutMatch — -L prints only names of files containing no matches.
//
// GNU command: ggrep -L match match.txt one.txt
// Expected:    "one.txt\n"   (only one.txt has no match)
func TestGNUCompatGrepFilesWithoutMatch(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{
			"match.txt": "match\nnomatch\nmatch\n",
			"one.txt":   "only one line\n",
		},
		"grep -L match match.txt one.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "one.txt\n", stdout)
}

// ── Case-insensitive matching ─────────────────────────────────────────────────

// TestGNUCompatGrepIgnoreCase — -i matches regardless of case.
//
// GNU command: ggrep -i ALPHA three.txt   (three.txt = "alpha\nbeta\ngamma\n")
// Expected:    "alpha\n"
func TestGNUCompatGrepIgnoreCase(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"grep -i ALPHA three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\n", stdout)
}

// ── Invert match ──────────────────────────────────────────────────────────────

// TestGNUCompatGrepInvert — -v prints lines that do NOT match.
//
// GNU command: ggrep -v match match.txt   (all lines contain "match" → no output)
// Expected:    ""  (exit 1 because no lines were printed)
func TestGNUCompatGrepInvert(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"match.txt": "match\nnomatch\nmatch\n"},
		"grep -v match match.txt",
	)
	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "", stdout)
}

// ── Word regexp ───────────────────────────────────────────────────────────────

// TestGNUCompatGrepWordRegexp — -w does not match partial words.
//
// GNU command: ggrep -w mat match.txt   (no whole-word "mat" in "match" or "nomatch")
// Expected:    ""  (exit 1)
func TestGNUCompatGrepWordRegexp(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"match.txt": "match\nnomatch\nmatch\n"},
		"grep -w mat match.txt",
	)
	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "", stdout)
}

// ── Line regexp ───────────────────────────────────────────────────────────────

// TestGNUCompatGrepLineRegexp — -x matches only whole lines.
//
// GNU command: ggrep -x match match.txt
// Expected:    "match\nmatch\n"  (only the two exact-"match" lines, not "nomatch")
func TestGNUCompatGrepLineRegexp(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"match.txt": "match\nnomatch\nmatch\n"},
		"grep -x match match.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "match\nmatch\n", stdout)
}

// ── Only-matching ─────────────────────────────────────────────────────────────

// TestGNUCompatGrepOnlyMatching — -o prints only the matched portion of each line.
//
// GNU command: ggrep -o 'a.*a' three.txt   (three.txt = "alpha\nbeta\ngamma\n")
// Expected:    "alpha\namma\n"
//   alpha → "alpha" (full match from first 'a' to last 'a' in greedy RE)
//   gamma → "amma"  (match starts at second 'a')
func TestGNUCompatGrepOnlyMatching(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		`grep -o 'a.*a' three.txt`,
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\namma\n", stdout)
}

// ── With-filename ─────────────────────────────────────────────────────────────

// TestGNUCompatGrepWithFilename — -H forces filename prefix even for a single file.
//
// GNU command: ggrep -H alpha three.txt
// Expected:    "three.txt:alpha\n"
func TestGNUCompatGrepWithFilename(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"grep -H alpha three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "three.txt:alpha\n", stdout)
}

// ── No-filename ───────────────────────────────────────────────────────────────

// TestGNUCompatGrepNoFilename — --no-filename suppresses prefix even for multiple files.
//
// GNU command: ggrep --no-filename . three.txt one.txt
//   (three.txt = "alpha\nbeta\ngamma\n"; one.txt = "only one line\n")
// Expected:    "alpha\nbeta\ngamma\nonly one line\n"
func TestGNUCompatGrepNoFilename(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{
			"three.txt": "alpha\nbeta\ngamma\n",
			"one.txt":   "only one line\n",
		},
		"grep --no-filename . three.txt one.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\nbeta\ngamma\nonly one line\n", stdout)
}

// ── Two files: default filename prefix ───────────────────────────────────────

// TestGNUCompatGrepTwoFilesPrefix — two files get filename: prefix by default.
//
// GNU command: ggrep alpha three.txt copy.txt   (both contain "alpha\nbeta\ngamma\n")
// Expected:    "three.txt:alpha\ncopy.txt:alpha\n"
func TestGNUCompatGrepTwoFilesPrefix(t *testing.T) {
	content := "alpha\nbeta\ngamma\n"
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": content, "copy.txt": content},
		"grep alpha three.txt copy.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "three.txt:alpha\ncopy.txt:alpha\n", stdout)
}

// ── Context: after ────────────────────────────────────────────────────────────

// TestGNUCompatGrepAfterContext — -A 1 prints 1 trailing context line.
//
// GNU command: ggrep -A 1 needle needle.txt
//   (needle.txt = "line1\nline2\nneedle\nline4\nline5\n")
// Expected:    "needle\nline4\n"
func TestGNUCompatGrepAfterContext(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"needle.txt": "line1\nline2\nneedle\nline4\nline5\n"},
		"grep -A 1 needle needle.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "needle\nline4\n", stdout)
}

// ── Context: before ───────────────────────────────────────────────────────────

// TestGNUCompatGrepBeforeContext — -B 1 prints 1 leading context line.
//
// GNU command: ggrep -B 1 needle needle.txt
//   (needle.txt = "line1\nline2\nneedle\nline4\nline5\n")
// Expected:    "line2\nneedle\n"
func TestGNUCompatGrepBeforeContext(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"needle.txt": "line1\nline2\nneedle\nline4\nline5\n"},
		"grep -B 1 needle needle.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "line2\nneedle\n", stdout)
}

// ── Context: symmetric ────────────────────────────────────────────────────────

// TestGNUCompatGrepContext — -C 1 prints 1 line of context on both sides.
//
// GNU command: ggrep -C 1 needle needle.txt
//   (needle.txt = "line1\nline2\nneedle\nline4\nline5\n")
// Expected:    "line2\nneedle\nline4\n"
func TestGNUCompatGrepContext(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"needle.txt": "line1\nline2\nneedle\nline4\nline5\n"},
		"grep -C 1 needle needle.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "line2\nneedle\nline4\n", stdout)
}

// ── No trailing newline preserved ─────────────────────────────────────────────

// TestGNUCompatGrepNoTrailingNewline — last line without terminator is still printed.
//
// GNU command: ggrep . nonl.txt   (nonl.txt = "no newline" — no trailing \n)
// Expected:    "no newline\n"  (grep always adds \n at end)
func TestGNUCompatGrepNoTrailingNewline(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"nonl.txt": "no newline"},
		"grep . nonl.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "no newline\n", stdout)
}

// ── Empty file ────────────────────────────────────────────────────────────────

// TestGNUCompatGrepEmptyFile — empty file produces no output, exit 1.
//
// GNU command: ggrep . empty.txt   (empty.txt = "")
// Expected:    ""  (exit 1 — no matches)
func TestGNUCompatGrepEmptyFile(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"empty.txt": ""},
		"grep . empty.txt",
	)
	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "", stdout)
}

// ── Quiet mode exit codes ─────────────────────────────────────────────────────

// TestGNUCompatGrepQuietMatch — -q exits 0 when a match is found, no output.
//
// GNU command: ggrep -q alpha three.txt; echo $?
// Expected:    exit 0, no stdout
func TestGNUCompatGrepQuietMatch(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"grep -q alpha three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatGrepQuietNoMatch — -q exits 1 when no match is found, no output.
//
// GNU command: ggrep -q ZZNOTFOUND three.txt; echo $?
// Expected:    exit 1, no stdout
func TestGNUCompatGrepQuietNoMatch(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"grep -q ZZNOTFOUND three.txt",
	)
	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "", stdout)
}

// ── Fixed strings ─────────────────────────────────────────────────────────────

// TestGNUCompatGrepFixedStrings — -F treats pattern as literal (no regex).
//
// GNU command: ggrep -F 'a.*b' three.txt   (no literal "a.*b" in "alpha\nbeta\ngamma\n")
// Expected:    ""  (exit 1)
func TestGNUCompatGrepFixedStringsNoMatch(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		`grep -F 'a.*b' three.txt`,
	)
	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatGrepFixedStringsMatch — -F matches literal regex metacharacters.
//
// GNU command: ggrep -F 'a.*b' re.txt   (re.txt = "a.*b\nalpha\n")
// Expected:    "a.*b\n"
func TestGNUCompatGrepFixedStringsMatch(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"re.txt": "a.*b\nalpha\n"},
		`grep -F 'a.*b' re.txt`,
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "a.*b\n", stdout)
}

// ── -n -H two files ───────────────────────────────────────────────────────────

// TestGNUCompatGrepLineNumberWithFilename — -n -H on two files: filename:linenum:line.
//
// GNU command: ggrep -nH alpha three.txt copy.txt   (both files = "alpha\nbeta\ngamma\n")
// Expected:    "three.txt:1:alpha\ncopy.txt:1:alpha\n"
func TestGNUCompatGrepLineNumberWithFilename(t *testing.T) {
	content := "alpha\nbeta\ngamma\n"
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"three.txt": content, "copy.txt": content},
		"grep -nH alpha three.txt copy.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "three.txt:1:alpha\ncopy.txt:1:alpha\n", stdout)
}

// ── Max count ─────────────────────────────────────────────────────────────────

// TestGNUCompatGrepMaxCount — -m 1 stops after the first match.
//
// GNU command: ggrep -m 1 . five.txt   (five.txt = "foo\nbar\nbaz\nqux\nquux\n")
// Expected:    "foo\n"
func TestGNUCompatGrepMaxCount(t *testing.T) {
	stdout, _, exitCode := gnuCompatGrepRun(t,
		map[string]string{"five.txt": "foo\nbar\nbaz\nqux\nquux\n"},
		"grep -m 1 . five.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "foo\n", stdout)
}

// ── Rejected flag ─────────────────────────────────────────────────────────────

// TestGNUCompatGrepRejectedRecursive — -r is rejected with exit 2 and a stderr message.
//
// GNU command: our builtin rejects -r; GNU grep would recurse.
// Expected:    exit 2, non-empty stderr
func TestGNUCompatGrepRejectedRecursive(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "hello\n"})
	_, stderr, exitCode := grepRun(t, "grep -r pattern .", dir)
	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "grep:")
}
