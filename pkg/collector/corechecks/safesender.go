// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// SafeSender implements aggregator.Sender, wrapping the methods to provide
// some additional safety checks.
//
// In particular, the methods taking `tags []string` are wrapped to copy the
// slice, as the aggregator may modify it in-place.
type safeSender struct {
	aggregator.Sender
}

var _ aggregator.Sender = &safeSender{}

func newSafeSender(sender aggregator.Sender) aggregator.Sender {
	return &safeSender{Sender: sender}
}

// cloneTags clones a []string of tags.
//
// The underlying sender methods take ownership of the []string, and might
// modify it in-place.  If the caller is still using that tags slice, this can
// cause corruption of tags.
func cloneTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	tagsCopy := make([]string, len(tags))
	copy(tagsCopy, tags)
	return tagsCopy
}

// Gauge implememnts aggregator.Sender#Gauge.
func (ss *safeSender) Gauge(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Gauge(metric, value, hostname, cloneTags(tags))
}

// Rate implememnts aggregator.Sender#Rate.
func (ss *safeSender) Rate(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Rate(metric, value, hostname, cloneTags(tags))
}

// Count implememnts aggregator.Sender#Count.
func (ss *safeSender) Count(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Count(metric, value, hostname, cloneTags(tags))
}

// MonotonicCount implememnts aggregator.Sender#MonotonicCount.
func (ss *safeSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	ss.Sender.MonotonicCount(metric, value, hostname, cloneTags(tags))
}

// MonotonicCountWithFlushFirstValue implememnts aggregator.Sender#MonotonicCountWithFlushFirstValue.
func (ss *safeSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	ss.Sender.MonotonicCountWithFlushFirstValue(metric, value, hostname, cloneTags(tags), flushFirstValue)
}

// Counter implememnts aggregator.Sender#Counter.
func (ss *safeSender) Counter(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Counter(metric, value, hostname, cloneTags(tags))
}

// Histogram implememnts aggregator.Sender#Histogram.
func (ss *safeSender) Histogram(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Histogram(metric, value, hostname, cloneTags(tags))
}

// Historate implememnts aggregator.Sender#Historate.
func (ss *safeSender) Historate(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Historate(metric, value, hostname, cloneTags(tags))
}

// ServiceCheck implememnts aggregator.Sender#ServiceCheck.
func (ss *safeSender) ServiceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string) {
	ss.Sender.ServiceCheck(checkName, status, hostname, cloneTags(tags), message)
}

// HistogramBucket implememnts aggregator.Sender#HistogramBucket.
func (ss *safeSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	ss.Sender.HistogramBucket(metric, value, lowerBound, upperBound, monotonic, hostname, cloneTags(tags), flushFirstValue)
}

// SetCheckCustomTags implememnts aggregator.Sender#SetCheckCustomTags.
func (ss *safeSender) SetCheckCustomTags(tags []string) {
	ss.Sender.SetCheckCustomTags(cloneTags(tags))
}
