// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

// SafeSender implements sender.Sender, wrapping the methods to provide
// some additional safety checks.
//
// In particular, the methods taking `tags []string` are wrapped to copy the
// slice, as the aggregator may modify it in-place.
type safeSender struct {
	sender.Sender
}

var _ sender.Sender = &safeSender{}

func newSafeSender(sender sender.Sender) sender.Sender {
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

// Gauge implements sender.Sender#Gauge.
func (ss *safeSender) Gauge(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Gauge(metric, value, hostname, cloneTags(tags))
}

// Rate implements sender.Sender#Rate.
func (ss *safeSender) Rate(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Rate(metric, value, hostname, cloneTags(tags))
}

// Count implements sender.Sender#Count.
func (ss *safeSender) Count(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Count(metric, value, hostname, cloneTags(tags))
}

// MonotonicCount implements sender.Sender#MonotonicCount.
func (ss *safeSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	ss.Sender.MonotonicCount(metric, value, hostname, cloneTags(tags))
}

// MonotonicCountWithFlushFirstValue implements sender.Sender#MonotonicCountWithFlushFirstValue.
func (ss *safeSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	ss.Sender.MonotonicCountWithFlushFirstValue(metric, value, hostname, cloneTags(tags), flushFirstValue)
}

// Counter implements sender.Sender#Counter.
func (ss *safeSender) Counter(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Counter(metric, value, hostname, cloneTags(tags))
}

// Histogram implements sender.Sender#Histogram.
func (ss *safeSender) Histogram(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Histogram(metric, value, hostname, cloneTags(tags))
}

// Historate implements sender.Sender#Historate.
func (ss *safeSender) Historate(metric string, value float64, hostname string, tags []string) {
	ss.Sender.Historate(metric, value, hostname, cloneTags(tags))
}

// ServiceCheck implements sender.Sender#ServiceCheck.
func (ss *safeSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
	ss.Sender.ServiceCheck(checkName, status, hostname, cloneTags(tags), message)
}

// HistogramBucket implements sender.Sender#HistogramBucket.
func (ss *safeSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	ss.Sender.HistogramBucket(metric, value, lowerBound, upperBound, monotonic, hostname, cloneTags(tags), flushFirstValue)
}

// SetCheckCustomTags implements sender.Sender#SetCheckCustomTags.
func (ss *safeSender) SetCheckCustomTags(tags []string) {
	ss.Sender.SetCheckCustomTags(cloneTags(tags))
}
