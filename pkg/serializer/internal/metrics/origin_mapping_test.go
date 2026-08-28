// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// MicroVM origin product/category/service IDs are a contract with the backend
// intake registry; an accidental change silently mis-attributes metrics.

func TestMetricSourceToOriginServiceMicroVM(t *testing.T) {
	cases := map[metrics.MetricSource]int32{
		metrics.MetricSourceAWSMicroVMCustom:   472,
		metrics.MetricSourceAWSMicroVMEnhanced: 473,
		metrics.MetricSourceAWSMicroVMRuntime:  474,
	}
	for source, want := range cases {
		t.Run(source.String(), func(t *testing.T) {
			assert.Equal(t, want, metricSourceToOriginService(source))
		})
	}
}

func TestMetricSourceToOriginCategoryMicroVM(t *testing.T) {
	const microVMCategory int32 = 90
	for _, source := range []metrics.MetricSource{
		metrics.MetricSourceAWSMicroVMCustom,
		metrics.MetricSourceAWSMicroVMEnhanced,
		metrics.MetricSourceAWSMicroVMRuntime,
	} {
		t.Run(source.String(), func(t *testing.T) {
			assert.Equal(t, microVMCategory, metricSourceToOriginCategory(source))
		})
	}
}

func TestMetricSourceToOriginProductMicroVMIsServerless(t *testing.T) {
	// MicroVM should map to the same origin product as other serverless platforms.
	want := metricSourceToOriginProduct(metrics.MetricSourceGoogleCloudRunCustom)
	for _, source := range []metrics.MetricSource{
		metrics.MetricSourceAWSMicroVMCustom,
		metrics.MetricSourceAWSMicroVMEnhanced,
		metrics.MetricSourceAWSMicroVMRuntime,
	} {
		t.Run(source.String(), func(t *testing.T) {
			assert.Equal(t, want, metricSourceToOriginProduct(source))
		})
	}
}
