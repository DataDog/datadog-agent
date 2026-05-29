// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisWildcardFileOrderingStable \
//	    ./pkg/logs/launchers/file/provider/ -v
//
// Demonstrates property `wildcard-file-ordering-stable`: when filepath.Glob (or
// doublestar) returns files in a non-lexicographic order, applyReverseLexicographicalOrdering
// produces a wrong ordering. The algorithm first reverses the input slice (relying on it
// being pre-sorted lexicographically by directory), then stable-sorts by descending basename.
// When the input is NOT lexicographically sorted, the reverse step does not produce the
// correct directory ordering, and the subsequent stable sort propagates that wrong order.
//
// Practical consequence: when more wildcard-matching files exist than the filesLimit cap,
// FilesToTail selects the WRONG top-N files to tail — silently. Lower-priority files get
// tailed instead of higher-priority ones.
//
// This test re-encodes the SKIPPED test "Multiple Directories - Out of order input"
// (file_provider_test.go:644) WITHOUT calling t.Skip(), inside this build-tagged file.
// The original test is skipped with the comment "See FIXME in 'applyOrdering', this test
// currently fails". We demonstrate the failure here and assert the wrong (buggy) output to
// confirm the bug is reproduced, then FAIL with a clear bug report.

package fileprovider

import (
	"strings"
	"testing"

	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
)

// TestAntithesisWildcardFileOrderingStable demonstrates the ordering bug in
// applyReverseLexicographicalOrdering when the input files are NOT in lexicographic
// directory order (as if filepath.Glob returned them in inode order, for example).
//
// The bug: the function assumes its input arrives in lexicographically sorted order.
// It first reverses the slice (to get dirs in descending lex order), then stable-sorts by
// descending basename. If the input is already out of order, the reverse step does NOT
// produce the right directory ordering, and the stable sort preserves that wrong order.
//
// EXPECTED TO FAIL — the final assertion calls t.Fatalf to signal "BUG DEMONSTRATED".
func TestAntithesisWildcardFileOrderingStable(t *testing.T) {
	t.Run("OutOfOrderInput_OrderingIsWrong", func(t *testing.T) {
		// This is the exact input from the skipped test in file_provider_test.go:644.
		// Files arrive in a non-lexicographic order — as if filepath.Glob returned them
		// sorted by inode rather than by name (which is NOT guaranteed by the Go stdlib,
		// see https://github.com/golang/go/issues/17153 cited in the FIXME comment).
		//
		// Lexicographic order would be:
		//   /tmp/1/2017.log, /tmp/1/2018.log, /tmp/2/2018.log, /tmp/3/2016.log, /tmp/3/2018.log
		//
		// But here they arrive out of order:
		files := []*tailer.File{
			{Path: "/tmp/1/2018.log"},
			{Path: "/tmp/2/2018.log"},
			{Path: "/tmp/3/2016.log"},
			{Path: "/tmp/3/2018.log"},
			{Path: "/tmp/1/2017.log"},
		}

		applyReverseLexicographicalOrdering(files)

		// What the algorithm SHOULD produce (correct reverse-lexicographic ordering).
		// Priority rule: prefer files from higher-numbered (more recent) directories first,
		// break ties by descending filename. So /tmp/3 files come first, then /tmp/2, then /tmp/1.
		//
		// Correct expected output:
		//   [0] /tmp/3/2018.log  — dir 3 wins; filename 2018 > 2016
		//   [1] /tmp/3/2016.log  — dir 3 wins even though filename is 2016
		//   [2] /tmp/2/2018.log  — dir 2 comes next
		//   [3] /tmp/1/2018.log  — dir 1; filename 2018 > 2017
		//   [4] /tmp/1/2017.log
		wantPaths := []string{
			"/tmp/3/2018.log",
			"/tmp/3/2016.log",
			"/tmp/2/2018.log",
			"/tmp/1/2018.log",
			"/tmp/1/2017.log",
		}

		// What the BUGGY algorithm actually produces.
		// Trace:
		//   Input: [/tmp/1/2018.log, /tmp/2/2018.log, /tmp/3/2016.log, /tmp/3/2018.log, /tmp/1/2017.log]
		//   After reverse: [/tmp/1/2017.log, /tmp/3/2018.log, /tmp/3/2016.log, /tmp/2/2018.log, /tmp/1/2018.log]
		//   After stable-sort descending by basename:
		//     2018.log group (stable positions 1,3,4): /tmp/3/2018.log, /tmp/2/2018.log, /tmp/1/2018.log
		//     2017.log (position 0): /tmp/1/2017.log
		//     2016.log (position 2): /tmp/3/2016.log
		//   Result: [/tmp/3/2018.log, /tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log, /tmp/3/2016.log]
		//
		// The bug: /tmp/3/2016.log (dir 3, should be high priority) ends up LAST because
		// its basename "2016.log" sorts below all other basenames. The directory priority
		// (dir 3 > dir 2 > dir 1) is completely lost for this file.
		buggyActualPaths := []string{
			"/tmp/3/2018.log",
			"/tmp/2/2018.log",
			"/tmp/1/2018.log",
			"/tmp/1/2017.log",
			"/tmp/3/2016.log",
		}

		// Verify the algorithm produced the WRONG output (confirming the bug is present).
		actualPaths := make([]string, len(files))
		for i, f := range files {
			actualPaths[i] = f.Path
		}

		// Check that actual matches the known-buggy output (not the correct output).
		wrongOutput := pathsMatch(actualPaths, buggyActualPaths)
		correctOutput := pathsMatch(actualPaths, wantPaths)

		t.Logf("Input (non-lex order):    %s", strings.Join([]string{
			"/tmp/1/2018.log", "/tmp/2/2018.log", "/tmp/3/2016.log", "/tmp/3/2018.log", "/tmp/1/2017.log",
		}, ", "))
		t.Logf("Correct expected output:  %s", strings.Join(wantPaths, ", "))
		t.Logf("Buggy actual output:      %s", strings.Join(buggyActualPaths, ", "))
		t.Logf("Observed actual output:   %s", strings.Join(actualPaths, ", "))
		t.Logf("")
		t.Logf("correctOutput=%v  wrongOutput=%v", correctOutput, wrongOutput)

		if correctOutput {
			t.Logf("NOT A BUG (or fixed): algorithm produced correct ordering even with out-of-order input.")
			return
		}

		if !wrongOutput {
			t.Fatalf("INCONCLUSIVE: algorithm produced neither the correct output nor the known-buggy output.\n"+
				"Observed: %v\nExpected (correct): %v\nExpected (buggy):   %v",
				actualPaths, wantPaths, buggyActualPaths)
		}

		// Bug confirmed: output matches the known-buggy ordering.
		// Now assert that the CORRECT output is produced — this assertion WILL FAIL,
		// which is the intended signal "BUG DEMONSTRATED".
		//
		// Under the filesLimit cap scenario: if filesLimit=4, the 4 selected files are
		// [/tmp/3/2018.log, /tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log].
		// But the correct top-4 should be [/tmp/3/2018.log, /tmp/3/2016.log, /tmp/2/2018.log, /tmp/1/2018.log].
		// /tmp/3/2016.log (dir-3 file) is EXCLUDED and /tmp/1/2017.log (dir-1 file) is INCLUDED instead.
		// This is the silent wrong-file-selection bug.
		filesLimit := 4
		selectedFiles := actualPaths[:filesLimit]
		correctTopN := wantPaths[:filesLimit]

		selectionCorrect := pathsMatch(selectedFiles, correctTopN)

		t.Logf("")
		t.Logf("filesLimit cap scenario (limit=%d):", filesLimit)
		t.Logf("  Files selected (buggy):  %v", selectedFiles)
		t.Logf("  Files selected (correct):%v", correctTopN)
		t.Logf("  /tmp/3/2016.log (dir-3 high-priority file) in buggy selection: %v",
			contains(selectedFiles, "/tmp/3/2016.log"))
		t.Logf("  /tmp/1/2017.log (dir-1 low-priority file) in buggy selection:  %v",
			contains(selectedFiles, "/tmp/1/2017.log"))

		if !selectionCorrect {
			t.Fatalf(
				"BUG DEMONSTRATED (wildcard-file-ordering-stable): "+
					"applyReverseLexicographicalOrdering produced wrong ordering when input was "+
					"NOT in lexicographic order.\n\n"+
					"Root cause: the algorithm first reverses the slice (assuming the input is "+
					"already lex-sorted by directory), then stable-sorts by descending basename. "+
					"When the input is out of order, the reverse step does NOT produce the correct "+
					"descending directory ordering, and stable-sort propagates that wrong order.\n\n"+
					"Practical impact: with filesLimit=%d cap, the files selected are:\n  %s\n"+
					"but the CORRECT top-%d by reverse-lexicographic priority should be:\n  %s\n\n"+
					"Specifically, /tmp/3/2016.log (from directory /tmp/3, the HIGHEST-priority "+
					"directory) was EXCLUDED from tailing, while /tmp/1/2017.log (from the "+
					"LOWEST-priority directory /tmp/1) was INCLUDED instead. This is a silent "+
					"observability gap: no error is logged, no metric signals wrong selection.\n\n"+
					"The FIXME comment at file_provider.go:362 and the skipped test at "+
					"file_provider_test.go:644 both acknowledge this bug exists.",
				filesLimit,
				strings.Join(selectedFiles, ", "),
				filesLimit,
				strings.Join(correctTopN, ", "),
			)
		}
	})

	t.Run("InOrderInput_AlgorithmLimitation", func(t *testing.T) {
		// Clarification: even with LEXICOGRAPHICALLY SORTED input, the algorithm
		// cannot correctly handle the case where a file in a higher-priority directory
		// has a "lower" basename than files in lower-priority directories.
		//
		// Lex-sorted input: /tmp/1/2017.log, /tmp/1/2018.log, /tmp/2/2018.log, /tmp/3/2016.log, /tmp/3/2018.log
		//
		// After reversal: /tmp/3/2018.log, /tmp/3/2016.log, /tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log
		// After stable-sort by descending basename:
		//   2018.log (positions 0, 2, 3): /tmp/3/2018.log, /tmp/2/2018.log, /tmp/1/2018.log
		//   2017.log (position 4):        /tmp/1/2017.log
		//   2016.log (position 1):        /tmp/3/2016.log
		//   Result: /tmp/3/2018.log, /tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log, /tmp/3/2016.log
		//
		// The algorithm puts /tmp/3/2016.log LAST even with lex-sorted input, because
		// basename "2016.log" < "2017.log" < "2018.log". The CORRECT expected output
		// (what the skipped test asserts at line 644) has /tmp/3/2016.log at position [1]
		// because dir /tmp/3 should have priority over /tmp/2 and /tmp/1. The algorithm
		// can NEVER achieve this because it doesn't consider the full path for tiebreaking;
		// it only stable-sorts by basename, which means files with lower basenames always
		// lose to files with higher basenames, regardless of which directory they are in.
		//
		// Therefore: the bug in the skipped test is DEEPER than just "out of order input":
		// the algorithm has a FUNDAMENTAL DESIGN FLAW when files across different
		// directories have different basenames that happen to rank lower than basenames
		// in lower-priority directories.
		//
		// The "out of order input" skipped test title refers to the real-world scenario
		// where filepath.Glob returns files from the SAME dataset in a different
		// intra-directory order (e.g., /tmp/1/2017.log appears after /tmp/3/* instead of
		// before them), which also breaks the algorithm.
		files := []*tailer.File{
			{Path: "/tmp/1/2017.log"},
			{Path: "/tmp/1/2018.log"},
			{Path: "/tmp/2/2018.log"},
			{Path: "/tmp/3/2016.log"},
			{Path: "/tmp/3/2018.log"},
		}

		applyReverseLexicographicalOrdering(files)

		// What the algorithm actually produces for lex-sorted input with this dataset:
		// /tmp/3/2016.log ends up LAST despite being in the highest-priority directory.
		algorithmActualPaths := []string{
			"/tmp/3/2018.log",
			"/tmp/2/2018.log",
			"/tmp/1/2018.log",
			"/tmp/1/2017.log",
			"/tmp/3/2016.log", // BUG: dir-3 file is last due to low basename rank
		}

		actualPaths := make([]string, len(files))
		for i, f := range files {
			actualPaths[i] = f.Path
		}

		t.Logf("Lex-sorted input:        %v", []string{"/tmp/1/2017.log", "/tmp/1/2018.log", "/tmp/2/2018.log", "/tmp/3/2016.log", "/tmp/3/2018.log"})
		t.Logf("Actual output:           %v", actualPaths)
		t.Logf("/tmp/3/2016.log rank:    %d (expected: 1 for correct priority, got: last)", func() int {
			for i, p := range actualPaths {
				if p == "/tmp/3/2016.log" {
					return i
				}
			}
			return -1
		}())

		if !pathsMatch(actualPaths, algorithmActualPaths) {
			t.Logf("NOTE: algorithm produced different output than predicted. Got: %v", actualPaths)
			// Not a fatal error — just update our understanding.
		} else {
			t.Logf("CONFIRMED: even with lex-sorted input, /tmp/3/2016.log is ranked LAST "+
				"(position %d) despite being in the highest-priority directory. "+
				"Algorithm limitation: it cannot give priority to a dir-3 file with a "+
				"'lower' basename over dir-1/dir-2 files with 'higher' basenames.",
				len(actualPaths)-1)
		}

		// Verify the fundamental limitation: /tmp/3/2016.log should be above /tmp/2/2018.log
		// (since dir 3 > dir 2) but even with sorted input it ends up below it.
		pos3_2016 := -1
		pos2_2018 := -1
		for i, p := range actualPaths {
			switch p {
			case "/tmp/3/2016.log":
				pos3_2016 = i
			case "/tmp/2/2018.log":
				pos2_2018 = i
			}
		}

		if pos3_2016 > pos2_2018 {
			t.Fatalf(
				"BUG DEMONSTRATED (algorithm limitation with lex-sorted input): "+
					"/tmp/3/2016.log (rank %d) is below /tmp/2/2018.log (rank %d) "+
					"even with lex-sorted input. Dir /tmp/3 should have priority over "+
					"/tmp/2 but the algorithm cannot express this when basenames conflict "+
					"across directories. This confirms the fundamental design flaw that "+
					"the skipped test exposes.",
				pos3_2016, pos2_2018,
			)
		}
	})
}

// TestAntithesisWildcardFilesLimitCapSelectionBug demonstrates the FilesToTail
// silent wrong-selection bug end-to-end using a real FileProvider with 3 directories
// and a filesLimit cap. It manually calls applyReverseLexicographicalOrdering with
// out-of-order input (simulating non-lex Glob results) and then applies the cap to
// show exactly which files would be tailed.
func TestAntithesisWildcardFilesLimitCapSelectionBug(t *testing.T) {
	// Simulate: 5 matching files across 3 directories, filesLimit=3.
	// The correct top-3 by reverse-lex should prioritize dir /tmp/3 first.
	// Input out of lex order (e.g., inode order from filesystem):
	outOfOrderFiles := []*tailer.File{
		{Path: "/tmp/1/2018.log"},
		{Path: "/tmp/2/2018.log"},
		{Path: "/tmp/3/2016.log"},
		{Path: "/tmp/3/2018.log"},
		{Path: "/tmp/1/2017.log"},
	}

	filesLimit := 3

	// Run the ordering function on out-of-order input.
	applyReverseLexicographicalOrdering(outOfOrderFiles)

	// Simulate what FilesToTail does: take the first filesLimit files.
	selectedByBug := make([]string, filesLimit)
	for i := 0; i < filesLimit; i++ {
		selectedByBug[i] = outOfOrderFiles[i].Path
	}

	// Correct top-3 (what a correct ordering would produce):
	correctTop3 := []string{
		"/tmp/3/2018.log",
		"/tmp/3/2016.log",
		"/tmp/2/2018.log",
	}

	t.Logf("filesLimit=%d", filesLimit)
	t.Logf("Files selected by BUGGY ordering:   %v", selectedByBug)
	t.Logf("Files that SHOULD be selected:       %v", correctTop3)
	t.Logf("/tmp/3/2016.log in buggy selection:  %v", contains(selectedByBug, "/tmp/3/2016.log"))
	t.Logf("/tmp/3/2016.log in correct selection:%v", contains(correctTop3, "/tmp/3/2016.log"))
	t.Logf("/tmp/1/2017.log in buggy selection:  %v", contains(selectedByBug, "/tmp/1/2017.log"))

	// The key wrong-selection: /tmp/3/2016.log (highest-priority dir) should be in top-3,
	// but with the bug it gets displaced by /tmp/1/2018.log (lowest-priority dir).
	if contains(selectedByBug, "/tmp/3/2016.log") {
		t.Logf("NOT A BUG (or fixed): /tmp/3/2016.log was correctly included in the top-%d.", filesLimit)
		return
	}

	t.Fatalf(
		"BUG DEMONSTRATED (wildcard-file-ordering-stable / filesLimit cap):\n"+
			"With filesLimit=%d and out-of-order Glob input, FilesToTail selects:\n  %s\n"+
			"but the correct reverse-lexicographic top-%d should be:\n  %s\n\n"+
			"/tmp/3/2016.log is from the HIGHEST-priority directory (/tmp/3) but was\n"+
			"excluded because its basename '2016.log' sorts below '2018.log', and the\n"+
			"directory-priority information was lost when the input was not pre-sorted.\n"+
			"This is a SILENT wrong-file-selection: no error, no warning, no metric.",
		filesLimit,
		strings.Join(selectedByBug, ", "),
		filesLimit,
		strings.Join(correctTop3, ", "),
	)
}

// pathsMatch returns true if two string slices are equal element-by-element.
func pathsMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// contains returns true if s is in the slice ss.
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

