// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flavor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHumanReadableFlavor(t *testing.T) {
	for k, v := range agentFlavors {
		t.Run(fmt.Sprintf("%s: %s", k, v), func(t *testing.T) {
			SetFlavor(k)

			assert.Equal(t, v, GetHumanReadableFlavor())
		})
	}

	t.Run("Unknown Agent", func(t *testing.T) {
		SetFlavor("foo")

		assert.Equal(t, "Unknown Agent", GetHumanReadableFlavor())
	})
}
