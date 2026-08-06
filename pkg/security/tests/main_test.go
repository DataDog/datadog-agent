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
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var GitAncestorOnMain = "main"

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()

	// Every config is registered at init() time, so the partition of the suite
	// into runs that each build the eBPF module once needs no test execution.
	// Keep this ahead of any other output: the KMT test runner parses stdout.
	if listTestRuns {
		if err := printTestRuns(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	fmt.Printf("Using git ref %s as common ancestor between HEAD and main branch\n", GitAncestorOnMain)

	preTestsHook()
	retCode := m.Run()
	postTestsHook()

	reportUndeclaredStaticOpts()
	if code := checkModuleBuilds(testRunName); code != 0 && retCode == 0 {
		retCode = code
	}

	if commonCfgDir != "" {
		_ = os.RemoveAll(commonCfgDir)
	}

	os.Exit(retCode)
}

var (
	commonCfgDir string

	logLevelStr     string
	logPatterns     stringSlice
	logTags         stringSlice
	ebpfLessEnabled bool
	listTestRuns    bool
	testRunName     string
)

func init() {
	flag.StringVar(&logLevelStr, "loglevel", log.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")
	flag.BoolVar(&listTestRuns, "cws-list-groups", false,
		"print how the suite partitions into runs that each build the eBPF module once, as one JSON object per line, then exit")
	flag.StringVar(&testRunName, "cws-group", "",
		"name of the run being executed, from -cws-list-groups; makes the suite fail if it builds more modules than that run predicted")
}
