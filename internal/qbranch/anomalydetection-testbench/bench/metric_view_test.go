// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewParquetMetricViewResolvesHostTag(t *testing.T) {
	view := newParquetMetricView("system.cpu", 1, []string{"env:prod", "host:web-1", "service:api"}, 100)

	assert.Equal(t, "web-1", view.GetHost())
	assert.Equal(t, []string{"env:prod", "service:api"}, view.GetTags().UnsafeToReadOnlySliceString())
}

func TestParseSeriesKeyIncludesHost(t *testing.T) {
	namespace, name, host, tags, ok := parseSeriesKey("parquet|system.cpu:avg|web-1|env:prod,service:api")

	assert.True(t, ok)
	assert.Equal(t, "parquet", namespace)
	assert.Equal(t, "system.cpu:avg", name)
	assert.Equal(t, "web-1", host)
	assert.Equal(t, []string{"env:prod", "service:api"}, tags)
}
