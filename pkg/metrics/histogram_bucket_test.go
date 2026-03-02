// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestHistogramBucketGetName(t *testing.T) {
	bucket := &HistogramBucket{
		Name: "test.histogram.bucket",
	}
	assert.Equal(t, "test.histogram.bucket", bucket.GetName())
}

func TestHistogramBucketGetHost(t *testing.T) {
	bucket := &HistogramBucket{
		Host: "test-host",
	}
	assert.Equal(t, "test-host", bucket.GetHost())
}

func TestHistogramBucketGetTags(t *testing.T) {
	bucket := &HistogramBucket{
		Tags: []string{"env:prod", "service:web"},
	}

	tb := tagset.NewHashingTagsAccumulator()
	bucket.GetTags(nil, tb, nil)

	tags := tb.Get()
	assert.Contains(t, tags, "env:prod")
	assert.Contains(t, tags, "service:web")
}

func TestHistogramBucketGetMetricType(t *testing.T) {
	bucket := &HistogramBucket{}
	assert.Equal(t, HistogramType, bucket.GetMetricType())
}

func TestHistogramBucketIsNoIndex(t *testing.T) {
	bucket := &HistogramBucket{}
	assert.False(t, bucket.IsNoIndex())
}

func TestHistogramBucketGetSource(t *testing.T) {
	bucket := &HistogramBucket{
		Source: MetricSourceDogstatsd,
	}
	assert.Equal(t, MetricSourceDogstatsd, bucket.GetSource())
}

func TestHistogramBucketFields(t *testing.T) {
	bucket := &HistogramBucket{
		Name:            "test.bucket",
		Value:           100,
		LowerBound:      0.0,
		UpperBound:      10.0,
		Monotonic:       true,
		Tags:            []string{"tag:value"},
		Host:            "hostname",
		Timestamp:       1234567890.0,
		FlushFirstValue: true,
		Source:          MetricSourceInternal,
	}

	assert.Equal(t, "test.bucket", bucket.Name)
	assert.Equal(t, int64(100), bucket.Value)
	assert.Equal(t, 0.0, bucket.LowerBound)
	assert.Equal(t, 10.0, bucket.UpperBound)
	assert.True(t, bucket.Monotonic)
	assert.Equal(t, []string{"tag:value"}, bucket.Tags)
	assert.Equal(t, "hostname", bucket.Host)
	assert.Equal(t, 1234567890.0, bucket.Timestamp)
	assert.True(t, bucket.FlushFirstValue)
	assert.Equal(t, MetricSourceInternal, bucket.Source)
}
