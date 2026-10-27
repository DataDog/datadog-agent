// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package compliance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/goleak"
)

// TestMain makes the test binary stand in for oscap-io, which the evaluator runs as
// `/proc/self/exe compliance oscap-exec <file>`.
func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "compliance" && os.Args[2] == "oscap-exec" {
		// the session directory OpenSCAP makes when it loads the datastream
		if _, err := os.MkdirTemp("", "oscap."); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		// a result, then an exit as abrupt as the libgcrypt aborts seen on FIPS hosts
		fmt.Println("xccdf_org.ssgproject.content_rule_test", XCCDF_RESULT_PASS)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestOSCAPIOReaderStopsWithProcess(t *testing.T) {
	ignoreExisting := goleak.IgnoreCurrent()

	p := newOSCAPIO(filepath.Join(t.TempDir(), "benchmark.xml"))
	// returns once oscap-io is gone, after its pipes and DoneCh are closed
	_ = p.Run(context.Background())

	// taking a result the reader is already blocked on sends it back to the closed pipe
	select {
	case <-p.ResultCh:
	default:
	}

	goleak.VerifyNone(t, ignoreExisting)
}

// TestOSCAPIOTempDirRemoved checks that the TMPDIR of an oscap-io process that
// exits abruptly, leaving its session directory, is removed with it.
func TestOSCAPIOTempDirRemoved(t *testing.T) {
	file := filepath.Join(t.TempDir(), "benchmark.xml")
	root := t.TempDir()
	t.Setenv("TMPDIR", root)

	p := newOSCAPIO(file)
	_ = p.Run(context.Background())

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("%s holds %s after oscap-io exited, want nothing", root, e.Name())
	}
}
