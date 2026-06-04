// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestCheckMetricSampleFactoryBuildsSenderMetricSampleFields(t *testing.T) {
	factory := NewCheckMetricSampleFactory(checkid.ID("cpu:instance"), "default-host", func() float64 { return 1234 })
	factory.SetCheckCustomTags([]string{"custom:tag"})
	factory.SetNoIndex(true)

	sample := factory.BuildMetricSample(ScalarSample{
		Name:            "system.cpu.user",
		Value:           42,
		Hostname:        "",
		Tags:            []string{"runtime:tag"},
		Type:            metrics.RateType,
		FlushFirstValue: true,
		NoIndex:         false,
		Timestamp:       0,
	})

	assert.Equal(t, "system.cpu.user", sample.Name)
	assert.Equal(t, 42.0, sample.Value)
	assert.Equal(t, metrics.RateType, sample.Mtype)
	assert.Equal(t, []string{"runtime:tag", "custom:tag"}, sample.Tags)
	assert.Equal(t, "default-host", sample.Host)
	assert.Equal(t, 1.0, sample.SampleRate)
	assert.Equal(t, 1234.0, sample.Timestamp)
	assert.True(t, sample.FlushFirstValue)
	assert.True(t, sample.NoIndex)
	assert.Equal(t, metrics.MetricSourceCPU, sample.Source)
}

func TestCheckMetricSampleFactoryPreservesExplicitHostnameTimestampAndNoIndex(t *testing.T) {
	factory := NewCheckMetricSampleFactory(checkid.ID("unknown:instance"), "default-host", func() float64 { return 1234 })

	sample := factory.BuildMetricSample(ScalarSample{
		Name:      "custom.metric",
		Value:     7,
		Hostname:  "submitted-host",
		Type:      metrics.GaugeWithTimestampType,
		NoIndex:   true,
		Timestamp: 1000,
	})

	assert.Equal(t, "submitted-host", sample.Host)
	assert.Equal(t, 1000.0, sample.Timestamp)
	assert.True(t, sample.NoIndex)
	assert.Equal(t, metrics.MetricSourceUnknown, sample.Source)
}

func TestCheckMetricSampleFactoryCanDisableDefaultHostname(t *testing.T) {
	factory := NewCheckMetricSampleFactory(checkid.ID("cpu:instance"), "default-host", func() float64 { return 1234 })
	factory.DisableDefaultHostname(true)

	sample := factory.BuildMetricSample(ScalarSample{
		Name:  "system.cpu.user",
		Value: 42,
		Type:  metrics.GaugeType,
	})

	assert.Empty(t, sample.Host)
}
