// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPseudoSet(t *testing.T) {
	elements := []string{"a", "a", "b", "b", "c", "c"}
	expected := []string{"a", "b", "c"}

	t.Run("elements are deduplicated", func(t *testing.T) {
		set := newPseudoSet[string]()
		for _, e := range elements {
			set.Add(e)
		}
		assert.ElementsMatch(t, expected, set.Slice())
	})
}
