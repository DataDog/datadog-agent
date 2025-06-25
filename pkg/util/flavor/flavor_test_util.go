// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package flavor

import (
	"testing"
)

// SetTestFlavor sets the flavor for the test
func SetTestFlavor(t *testing.T, flavor string) {
	originalFlavor := GetFlavor()
	SetFlavor(flavor)
	t.Cleanup(func() {
		SetFlavor(originalFlavor)
	})
}
