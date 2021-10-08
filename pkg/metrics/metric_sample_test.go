// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricSampleCopy(t *testing.T) {
	src := &MetricSample{}
	src.Host = "foo"
	src.Mtype = HistogramType
	src.Name = "metric.name"
	src.RawValue = "0.1"
	src.SampleRate = 1
	src.Tags = []string{"a:b", "c:d"}
	src.Timestamp = 1234
	src.Value = 0.1
	dst := src.Copy()

	assert.False(t, src == dst)
	assert.True(t, reflect.DeepEqual(&src, &dst))
}
