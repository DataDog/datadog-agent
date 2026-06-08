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

// BaseSender contains check sender identity behavior shared by concrete sender
// implementations. It does not own any output path.
type BaseSender struct {
	checkID                 checkid.ID
	defaultHostname         string
	now                     func() float64
	defaultHostnameDisabled bool
	checkTags               []string
	service                 string
	noIndex                 bool
}

// NewBaseSender creates the common sender state for a check sender.
func NewBaseSender(checkID checkid.ID, defaultHostname string, now func() float64) *BaseSender {
	return &BaseSender{
		checkID:         checkID,
		defaultHostname: defaultHostname,
		now:             now,
	}
}

// CheckID returns the check ID associated with the sender.
func (s *BaseSender) CheckID() checkid.ID {
	return s.checkID
}

// DisableDefaultHostname controls whether an empty submitted hostname is
// replaced with the sender's default hostname.
func (s *BaseSender) DisableDefaultHostname(disable bool) {
	s.defaultHostnameDisabled = disable
}

// SetCheckCustomTags stores tags from check configuration.
func (s *BaseSender) SetCheckCustomTags(tags []string) {
	s.checkTags = tags
}

// SetCheckService stores the service tag to apply at finalization time.
func (s *BaseSender) SetCheckService(service string) {
	s.service = service
}

// SetNoIndex controls whether formatted samples are marked no-index.
func (s *BaseSender) SetNoIndex(noIndex bool) {
	s.noIndex = noIndex
}

// FinalizeCheckServiceTag appends the configured service tag to check tags.
func (s *BaseSender) FinalizeCheckServiceTag() {
	if s.service != "" {
		s.checkTags = append(s.checkTags, "service:"+s.service)
	}
}

// CheckTags returns tags configured for the check sender.
func (s *BaseSender) CheckTags() []string {
	return s.checkTags
}

// Hostname returns the submitted hostname or the sender's default hostname.
func (s *BaseSender) Hostname(hostname string) string {
	if hostname == "" && !s.defaultHostnameDisabled {
		return s.defaultHostname
	}
	return hostname
}

// BuildMetricSample returns a MetricSample from scalar sender input.
func (s *BaseSender) BuildMetricSample(input ScalarSample) *metrics.MetricSample {
	tags := append(input.Tags, s.checkTags...)
	timestamp := input.Timestamp
	if timestamp == 0 {
		timestamp = s.now()
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
		NoIndex:         s.noIndex || input.NoIndex,
		Source:          metrics.CheckNameToMetricSource(checkid.IDToCheckName(s.checkID)),
	}

	sample.Host = s.Hostname(input.Hostname)

	return sample
}
