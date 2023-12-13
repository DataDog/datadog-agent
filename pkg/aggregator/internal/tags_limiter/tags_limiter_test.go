// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags_limiter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagsLimiter(t *testing.T) {
	const limit int = 5
	l := New(limit)
	for n := 0; n <= 10; n += 2 {
		for i := 0; i <= n; i++ {
			taggerTags := make([]string, i)
			metricTags := make([]string, n-i)
			assert.Equal(t, n < limit, l.Check(0, taggerTags, metricTags))
		}
	}
}
