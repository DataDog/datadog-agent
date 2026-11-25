// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package redact

import (
	"slices"
	"testing"
)

// RestoreSensitiveAnnotationsAndLabelsForTest restores the list of sensitive annotations and labels when the test
// ends, as UpdateSensitiveAnnotationsAndLabels only appends to it.
func RestoreSensitiveAnnotationsAndLabelsForTest(t testing.TB) {
	sensitiveAnnotationsAndLabelsMutex.Lock()
	saved := slices.Clone(sensitiveAnnotationsAndLabels)
	sensitiveAnnotationsAndLabelsMutex.Unlock()
	t.Cleanup(func() {
		sensitiveAnnotationsAndLabelsMutex.Lock()
		defer sensitiveAnnotationsAndLabelsMutex.Unlock()
		sensitiveAnnotationsAndLabels = saved
	})
}
