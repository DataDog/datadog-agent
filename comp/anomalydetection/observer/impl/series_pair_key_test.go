// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeriesPairKeyCanonicalization(t *testing.T) {
	a := "cpu.user:avg"
	b := "mem.used:avg"

	k1 := newSeriesPairKey(a, b)
	k2 := newSeriesPairKey(b, a)

	assert.Equal(t, k1, k2)
	assert.Equal(t, a, k1.A)
	assert.Equal(t, b, k1.B)
}

func TestSeriesPairKeyHashKeyAvoidsDelimiterAmbiguity(t *testing.T) {
	k1 := newSeriesPairKey("cpu.user:avg", "mem.used:avg")
	k2 := newSeriesPairKey("disk.io:count", "net.tx:sum")

	assert.NotEqual(t, k1.hashKey(), k2.hashKey())
}
