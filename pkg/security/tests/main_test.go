// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests

// Package tests holds tests related files
package tests

import (
	"flag"
	"os"
	"testing"
)

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()
	retCode := m.Run()
	if testMod != nil {
		testMod.cleanup()
	}

	if commonCfgDir != "" {
		_ = os.RemoveAll(commonCfgDir)
	}
	os.Exit(retCode)
}
