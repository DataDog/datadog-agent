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
	"time"

	"go.uber.org/goleak"
)

// TestMain makes the test binary stand in for oscap-io, which the evaluator runs as
// `/proc/self/exe compliance oscap-exec <file>`.
func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "compliance" && os.Args[2] == "oscap-exec" {
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

	// a result received this late sends the reader back to the closed pipe
	select {
	case <-p.ResultCh:
	case <-time.After(100 * time.Millisecond):
	}

	goleak.VerifyNone(t, ignoreExisting)
}
