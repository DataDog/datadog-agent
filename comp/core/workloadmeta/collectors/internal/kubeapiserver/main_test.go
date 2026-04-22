// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"os"
	"testing"
)

// TestMain disables the WatchListClient feature gate for the entire package.
// In client-go v0.35.3, WatchListClient is enabled by default but
// fake.NewSimpleClientset does not support the watch-list protocol (it never
// sends the required initial bookmark event), causing the reflector to hang.
// This must be done in TestMain rather than per-test because callers use
// t.Parallel() and SetFeatureDuringTest would conflict across parallel tests.
func TestMain(m *testing.M) {
	os.Setenv("KUBE_FEATURE_WatchListClient", "false")
	os.Exit(m.Run())
}
