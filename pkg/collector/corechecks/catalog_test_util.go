// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package corechecks

import "testing"

// WithTestCatalog replaces the global core-check catalog for the duration of a test.
func WithTestCatalog(t testing.TB) {
	t.Helper()
	originalCatalog := catalog
	catalog = make(map[string]ContextualCheckFactory)
	t.Cleanup(func() {
		catalog = originalCatalog
	})
}
