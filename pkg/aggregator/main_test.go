// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build test

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	checker := testutil.NewConfigChangeChecker()
	exit := m.Run()
	if checker.HasChanged() {
		os.Exit(1)
	}
	os.Exit(exit)
}
