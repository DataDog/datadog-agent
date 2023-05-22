// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import "testing"

func TestSubmitMetric(t *testing.T) {
	testSubmitMetric(t)
}

func TestSubmitMetricEmptyTags(t *testing.T) {
	testSubmitMetricEmptyTags(t)
}

func TestSubmitMetricEmptyHostname(t *testing.T) {
	testSubmitMetricEmptyHostname(t)
}

func TestSubmitServiceCheck(t *testing.T) {
	testSubmitServiceCheck(t)
}

func TestSubmitServiceCheckEmptyTag(t *testing.T) {
	testSubmitServiceCheckEmptyTag(t)
}

func TestSubmitServiceCheckEmptyHostame(t *testing.T) {
	testSubmitServiceCheckEmptyHostame(t)
}

func TestSubmitEvent(t *testing.T) {
	testSubmitEvent(t)
}

func TestSubmitHistogramBucket(t *testing.T) {
	testSubmitHistogramBucket(t)
}

func TestSubmitEventPlatformEvent(t *testing.T) {
	testSubmitEventPlatformEvent(t)
}
