// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests

// Package tests holds tests related files
package tests

import (
	"flag"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/cihub/seelog"
)

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()

	preTestsHook()
	retCode := m.Run()
	postTestsHook()

	if commonCfgDir != "" {
		_ = os.RemoveAll(commonCfgDir)
	}

	os.Exit(retCode)
}

var (
	logLevelStr     string
	logPatterns     stringSlice
	logTags         stringSlice
	ebpfLessEnabled bool
)

func init() {
	flag.StringVar(&logLevelStr, "loglevel", seelog.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")
	flag.BoolVar(&ebpfLessEnabled, "ebpfless", false, "enabled the ebpfless mode")

	rand.Seed(time.Now().UnixNano())
}
