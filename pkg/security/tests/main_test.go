// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var GitAncestorOnMain = "main"

var wasRequiredFailed = false

const (
	ExitCodePass = iota
	ExitCodeError
	ExitCodeWarning = 44 // this value is arbitrary
)

var requiredTests = []string{
	"TestChmod",
}

func isRequiredTest(testName string) bool {
	for _, required := range requiredTests {
		if strings.HasPrefix(testName, required) {
			return true
		}
	}
	return false
}

// TrackRequiredTestFailure tracks the failure of a required test
func TrackRequiredTestFailure(tb testing.TB) {
	tb.Helper()

	t, ok := tb.(*testing.T)
	if !ok {
		return
	}

	testName := t.Name()

	t.Cleanup(func() {
		if t.Failed() && isRequiredTest(testName) {
			fmt.Fprintf(os.Stderr, "Required test %s failed\n", testName)
			wasRequiredFailed = true
		}
	})
}

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()

	fmt.Printf("Using git ref %s as common ancestor between HEAD and main branch\n", GitAncestorOnMain)

	preTestsHook()
	retCode := m.Run()
	retCode = checkRetCode(retCode)
	postTestsHook()

	if commonCfgDir != "" {
		_ = os.RemoveAll(commonCfgDir)
	}
	// Write special marker to stdout for test-runner to parse (gotestsum passes stdout through)
	fmt.Printf("\n###TEST_EXIT_CODE:%d###\n", retCode)

	os.Exit(retCode)
}
func checkRetCode(retCode int) int {
	if retCode == ExitCodePass {
		return retCode
	}

	if wasRequiredFailed {
		return ExitCodeError
	}
	return ExitCodeWarning
}

var (
	commonCfgDir string

	logLevelStr     string
	logPatterns     stringSlice
	logTags         stringSlice
	ebpfLessEnabled bool
)

func init() {
	flag.StringVar(&logLevelStr, "loglevel", log.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")
}
