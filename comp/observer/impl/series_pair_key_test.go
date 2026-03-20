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
	a := observer.SeriesRef(0)
	b := observer.SeriesRef(1)

	k1 := newSeriesPairKey(a, b)
	k2 := newSeriesPairKey(b, a)

	assert.Equal(t, k1, k2)
	assert.Equal(t, a, k1.A)
	assert.Equal(t, b, k1.B)
}

func TestSeriesPairKeyHashKeyAvoidsDelimiterAmbiguity(t *testing.T) {
	// These pairs should have different hash keys since they are different refs
	k1 := newSeriesPairKey(observer.SeriesRef(0), observer.SeriesRef(1))
	k2 := newSeriesPairKey(observer.SeriesRef(2), observer.SeriesRef(3))

	assert.NotEqual(t, k1.hashKey(), k2.hashKey())
}
