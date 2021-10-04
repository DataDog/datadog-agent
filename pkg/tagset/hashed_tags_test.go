// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashedTagsSlice(t *testing.T) {
	ht := NewHashedTagsFromSlice([]string{"a", "b", "c", "d", "e"})
	ht2 := ht.Slice(1, 3)
	assert.Equal(t, ht2.Get(), []string{"b", "c"})
	assert.Equal(t, ht2.Hashes(), ht.Hashes()[1:3])
}
