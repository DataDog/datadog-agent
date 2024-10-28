// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package http

import "testing"

func setupTest(t *testing.T) {
	t.Cleanup(func() {
		noProxyIgnoredWarningMap = make(map[string]bool)
		noProxyUsedInFuture = make(map[string]bool)
		noProxyChanged = make(map[string]bool)
	})
}

// MockWarnings mocks the warnings with provided values
func MockWarnings(t *testing.T, ignored, usedInFuture, proxyChanged []string) {
	setupTest(t)

	for _, w := range ignored {
		noProxyIgnoredWarningMap[w] = true
	}
	for _, w := range usedInFuture {
		noProxyUsedInFuture[w] = true
	}
	for _, w := range proxyChanged {
		noProxyChanged[w] = true
	}
}
