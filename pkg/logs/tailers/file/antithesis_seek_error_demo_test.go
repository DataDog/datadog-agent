// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisSeekErrorOffsetRegression \
//	    ./pkg/logs/tailers/file/ -v
//
// Demonstrates property `offset-no-regression-on-seek-error`: when the lseek that
// positions a resuming tailer at its saved offset fails (a filesystem fault),
// `tailer_nix.go` does `ret, _ := f.Seek(offset, whence)` — discarding the error and
// storing `ret` (0) as the read offset. The tailer re-reads the file from the start,
// re-delivering every already-consumed line (an unbounded duplicate storm). The
// experiment resumes from a mid-file offset behind a Seek-failing file and the test
// ASSERTS no line before the resume offset is re-delivered. EXPECTED TO FAIL.
//
// The fault injection and experiment live in antithesis_demo_support.go so the SUT
// harness (pkg/logs/cmd/antithesis-rotation-demo) shares the exact same code.

package file

import (
	"testing"
)

func TestAntithesisSeekErrorOffsetRegression(t *testing.T) {
	res, err := RunSeekErrorExperiment(t.TempDir())
	if err != nil {
		t.Fatalf("experiment error: %v", err)
	}
	t.Logf("resumed from line %d; delivered %d messages; first=%q",
		res.ResumeLine, res.DeliveredCount, res.FirstLine)

	if res.Regressed {
		t.Fatalf("BUG DEMONSTRATED (offset-no-regression-on-seek-error): seek failed, the "+
			"error was discarded, offset regressed to 0, and already-consumed lines "+
			"(L00000..) were re-delivered. Resume from line %d was ignored.", res.ResumeLine)
	}
}
