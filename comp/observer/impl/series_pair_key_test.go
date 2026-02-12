// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
)

func TestSeriesPairKeyCanonicalization(t *testing.T) {
	a := observer.SeriesID("parquet|cpu.user:avg|host:A")
	b := observer.SeriesID("parquet|cpu.user:avg|host:B")

	k1 := newSeriesPairKey(a, b)
	k2 := newSeriesPairKey(b, a)

	assert.Equal(t, k1, k2)
	assert.Equal(t, a, k1.A)
	assert.Equal(t, b, k1.B)
}

func TestSeriesPairKeyHashKeyAvoidsDelimiterAmbiguity(t *testing.T) {
	// These pairs can collide under naive delimiter concatenation.
	k1 := newSeriesPairKey(observer.SeriesID("a|b"), observer.SeriesID("c"))
	k2 := newSeriesPairKey(observer.SeriesID("a"), observer.SeriesID("b|c"))

	assert.NotEqual(t, k1.hashKey(), k2.hashKey())
}
