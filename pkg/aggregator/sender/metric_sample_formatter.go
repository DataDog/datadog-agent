// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// ScalarSample contains the sender input needed to format a MetricSample.
type ScalarSample struct {
	Name            string
	Value           float64
	Hostname        string
	Tags            []string
	Type            metrics.MetricType
	FlushFirstValue bool
	NoIndex         bool
	Timestamp       float64
}

// CheckMetricSampleFormatter formats MetricSample values with check sender identity
// behavior shared by normal and lookback senders.
type CheckMetricSampleFormatter struct {
	checkID                 checkid.ID
	defaultHostname         string
	now                     func() float64
	defaultHostnameDisabled bool
	checkTags               []string
	noIndex                 bool
}

// NewCheckMetricSampleFormatter creates a formatter for scalar check metric samples.
func NewCheckMetricSampleFormatter(checkID checkid.ID, defaultHostname string, now func() float64) *CheckMetricSampleFormatter {
	return &CheckMetricSampleFormatter{
		checkID:         checkID,
		defaultHostname: defaultHostname,
		now:             now,
	}
}

// DisableDefaultHostname controls whether an empty submitted hostname is
// replaced with the formatter's default hostname.
func (f *CheckMetricSampleFormatter) DisableDefaultHostname(disable bool) {
	f.defaultHostnameDisabled = disable
}

// SetCheckCustomTags stores tags from check configuration.
func (f *CheckMetricSampleFormatter) SetCheckCustomTags(tags []string) {
	f.checkTags = tags
}

// SetNoIndex controls whether formatted samples are marked no-index.
func (f *CheckMetricSampleFormatter) SetNoIndex(noIndex bool) {
	f.noIndex = noIndex
}

// Format returns a MetricSample from scalar sender input.
func (f *CheckMetricSampleFormatter) Format(input ScalarSample) *metrics.MetricSample {
	tags := append(input.Tags, f.checkTags...)
	timestamp := input.Timestamp
	if timestamp == 0 {
		timestamp = f.now()
	}

	sample := &metrics.MetricSample{
		Name:            input.Name,
		Value:           input.Value,
		Mtype:           input.Type,
		Tags:            tags,
		Host:            input.Hostname,
		SampleRate:      1,
		Timestamp:       timestamp,
		FlushFirstValue: input.FlushFirstValue,
		NoIndex:         f.noIndex || input.NoIndex,
		Source:          metrics.CheckNameToMetricSource(checkid.IDToCheckName(f.checkID)),
	}

	if input.Hostname == "" && !f.defaultHostnameDisabled {
		sample.Host = f.defaultHostname
	}

	return sample
}
