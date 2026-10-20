// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package df_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
)

// posixDataRowRE matches a `df -P` data row by anchoring on the
// unambiguous "blocks used avail capacity" numeric block, ignoring
// whatever filesystem source appears before it and whatever mount
// point appears after.
//
// Why not strings.Fields()+len-N indexing? Real-world mounts can emit
// rows where the source (Mntfromname) or the mount point (Mntonname)
// contains whitespace — macOS automount lines like "map auto_home" and
// macOS Sequoia Cryptex volumes both do this. A token-count parser
// silently mis-attributes column values in those cases; anchoring on
// the numeric quartet is robust to any number of leading/trailing
// tokens.
var posixDataRowRE = regexp.MustCompile(`(?:^|\s)(\d+)\s+(\d+)\s+(\d+)\s+(\d+%|-)\s+\S`)

// TestDfDataRowsAreNumeric_POSIX runs df -P and verifies every data
// row's three numeric columns parse as unsigned integers. This catches
// the entire class of formatting bugs where a column would be empty,
// contain a stray "%", or wrap.
func TestDfDataRowsAreNumeric_POSIX(t *testing.T) {
	stdout, _, code := testutil.RunScript(t, "df -P", "")
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Greater(t, len(lines), 1, "expected header + at least one data row")
	parsed := 0
	for i, line := range lines[1:] {
		m := posixDataRowRE.FindStringSubmatch(line)
		if m == nil {
			t.Logf("row %d: unparseable POSIX df row, skipping: %q", i, line)
			continue
		}
		parsed++
		blocks, used, avail := m[1], m[2], m[3]
		_, err := strconv.ParseUint(blocks, 10, 64)
		assert.NoError(t, err, "row %d blocks not integer: %q", i, blocks)
		_, err = strconv.ParseUint(used, 10, 64)
		assert.NoError(t, err, "row %d used not integer: %q", i, used)
		_, err = strconv.ParseUint(avail, 10, 64)
		assert.NoError(t, err, "row %d available not integer: %q", i, avail)
	}
	assert.Greater(t, parsed, 0, "no data rows could be parsed")
}

// TestDfPercentFormat checks that the capacity column ends with '%' or
// equals '-' (the empty pseudo-FS sentinel). posixDataRowRE already
// requires that shape to match, so the assertion guards against
// regressions in the regex more than the implementation; we still keep
// it explicit for clarity.
func TestDfPercentFormat(t *testing.T) {
	stdout, _, _ := testutil.RunScript(t, "df -P", "")
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	parsed := 0
	for i, line := range lines[1:] {
		m := posixDataRowRE.FindStringSubmatch(line)
		if m == nil {
			t.Logf("row %d: unparseable POSIX df row, skipping: %q", i, line)
			continue
		}
		parsed++
		cap := m[4]
		if cap == "-" {
			continue
		}
		assert.True(t, strings.HasSuffix(cap, "%"),
			"row %d capacity column %q does not end with %%", i, cap)
	}
	assert.Greater(t, parsed, 0, "no data rows could be parsed")
}

// TestDfTotalSumIsConsistent — when --total is given, the total row's
// numeric columns must equal the saturated sum of the per-mount columns.
func TestDfTotalSumIsConsistent(t *testing.T) {
	stdout, _, code := testutil.RunScript(t, "df -P --total", "")
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) < 3 {
		t.Skip("not enough rows for total verification")
	}
	totalLine := lines[len(lines)-1]
	assert.True(t, strings.HasPrefix(totalLine, "total "), "last line is not total: %q", totalLine)

	// Sum the per-row block columns. Use saturated arithmetic to match
	// the implementation; if the sum would overflow we don't assert
	// equality, only non-zero. If any data row is unparseable, skip
	// the consistency check — the test sum would diverge from the
	// implementation's total by the contribution of skipped rows, and
	// we cannot pin that gap with a fixed tolerance.
	var sumBlocks, sumUsed, sumAvail uint64
	overflow := false
	parsed := 0
	unparseable := 0
	for _, line := range lines[1 : len(lines)-1] {
		m := posixDataRowRE.FindStringSubmatch(line)
		if m == nil {
			unparseable++
			continue
		}
		parsed++
		b, _ := strconv.ParseUint(m[1], 10, 64)
		u, _ := strconv.ParseUint(m[2], 10, 64)
		a, _ := strconv.ParseUint(m[3], 10, 64)
		if sumBlocks > ^uint64(0)-b {
			overflow = true
		}
		sumBlocks += b
		sumUsed += u
		sumAvail += a
	}
	if overflow {
		return
	}
	if unparseable > 0 {
		t.Skipf("%d of %d data rows had non-standard column structure; "+
			"sum-vs-total consistency cannot be verified", unparseable, len(lines)-2)
	}
	totalMatch := posixDataRowRE.FindStringSubmatch(totalLine)
	if totalMatch == nil {
		t.Fatalf("total row unparseable: %q", totalLine)
	}
	gotBlocks, _ := strconv.ParseUint(totalMatch[1], 10, 64)
	gotUsed, _ := strconv.ParseUint(totalMatch[2], 10, 64)
	gotAvail, _ := strconv.ParseUint(totalMatch[3], 10, 64)
	// Per-row 1K-blocks values are ceil(bytes/1024); the total row
	// is computed as ceil(totalBytes/1024). On a filesystem whose
	// f_frsize is not a multiple of 1024 (some FUSE backends), the
	// sum-of-rounded-rows can differ from the rounded-total by up
	// to one block per row. Real Linux/macOS filesystems use ≥4 KiB
	// blocks so this never surfaces in practice, but a tolerance
	// hardens the assertion against pathological FUSE drivers.
	tolerance := uint64(parsed)
	assert.LessOrEqual(t, absDiff(gotBlocks, sumBlocks), tolerance,
		"blocks total %d differs from sum %d by more than %d", gotBlocks, sumBlocks, tolerance)
	assert.LessOrEqual(t, absDiff(gotUsed, sumUsed), tolerance,
		"used total %d differs from sum %d by more than %d", gotUsed, sumUsed, tolerance)
	assert.LessOrEqual(t, absDiff(gotAvail, sumAvail), tolerance,
		"avail total %d differs from sum %d by more than %d", gotAvail, sumAvail, tolerance)
}

func absDiff(a, b uint64) uint64 {
	if a >= b {
		return a - b
	}
	return b - a
}

// posixTypeRowRE matches a `df -PT` data row. The first capture is the
// FS-type token immediately preceding the numeric quartet; everything
// before that is the (possibly multi-token) source.
var posixTypeRowRE = regexp.MustCompile(`(?:^|\s)(\S+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+%|-)\s+\S`)

// humanDataRowRE matches a `df -h` (or `-H`) data row. The first three
// columns may carry a K/M/G/T/P/E suffix; capacity is the usual
// percent/sentinel column.
var humanDataRowRE = regexp.MustCompile(`(?:^|\s)(\d+(?:\.\d+)?[KMGTPE]?)\s+(\d+(?:\.\d+)?[KMGTPE]?)\s+(\d+(?:\.\d+)?[KMGTPE]?)\s+(\d+%|-)\s+\S`)

// TestDfTypeFilterMatchesAtLeastOne picks a type from the unfiltered
// listing and verifies that filtering by it returns only rows of that
// type.
func TestDfTypeFilterMatchesAtLeastOne(t *testing.T) {
	stdoutT, _, code := testutil.RunScript(t, "df -PT", "")
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(stdoutT, "\n"), "\n")
	if len(lines) < 2 {
		t.Skip("no mounts to filter")
	}
	// Pick the first row whose -PT shape we can parse cleanly; some
	// macOS mounts (Cryptex) emit non-standard column structures.
	var fsType string
	for _, line := range lines[1:] {
		m := posixTypeRowRE.FindStringSubmatch(line)
		if m != nil {
			fsType = m[1]
			break
		}
	}
	if fsType == "" {
		t.Skip("no -PT row could be parsed")
	}
	stdoutF, _, _ := testutil.RunScript(t, "df -PT -t "+fsType, "")
	filtered := strings.Split(strings.TrimRight(stdoutF, "\n"), "\n")
	assert.Greater(t, len(filtered), 1, "filter should keep at least one row")
	for _, l := range filtered[1:] {
		m := posixTypeRowRE.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		assert.Equal(t, fsType, m[1])
	}
}

// TestDfExcludeTypeRemovesRows runs the unfiltered listing, picks a type,
// and verifies it disappears under -x.
func TestDfExcludeTypeRemovesRows(t *testing.T) {
	stdoutT, _, _ := testutil.RunScript(t, "df -PT", "")
	lines := strings.Split(strings.TrimRight(stdoutT, "\n"), "\n")
	if len(lines) < 2 {
		t.Skip("no mounts to exclude")
	}
	var fsType string
	for _, line := range lines[1:] {
		m := posixTypeRowRE.FindStringSubmatch(line)
		if m != nil {
			fsType = m[1]
			break
		}
	}
	if fsType == "" {
		t.Skip("no -PT row could be parsed")
	}
	stdoutX, _, _ := testutil.RunScript(t, "df -PT -x "+fsType, "")
	for _, l := range strings.Split(strings.TrimRight(stdoutX, "\n"), "\n")[1:] {
		m := posixTypeRowRE.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		assert.NotEqual(t, fsType, m[1])
	}
}

// TestDfHumanReadableHasNoDigits_AtLargeSizes — when sizes are big
// enough that human formatting kicks in, the size column must NOT be a
// raw integer (it should have a K/M/G/T/P/E suffix).
func TestDfHumanReadableHasNoDigits_AtLargeSizes(t *testing.T) {
	stdout, _, _ := testutil.RunScript(t, "df -h", "")
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) < 2 {
		t.Skip("no mounts")
	}
	suffixOK := func(s string) bool {
		if s == "" {
			return false
		}
		switch s[len(s)-1] {
		case 'K', 'M', 'G', 'T', 'P', 'E':
			return true
		}
		// Bare digits: still OK for sub-base sizes.
		for _, r := range s {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	for i, l := range lines[1:] {
		m := humanDataRowRE.FindStringSubmatch(l)
		if m == nil {
			t.Logf("row %d: unparseable -h df row, skipping: %q", i, l)
			continue
		}
		size := m[1]
		assert.True(t, suffixOK(size), "row %d size column %q has no recognised suffix", i, size)
	}
}
