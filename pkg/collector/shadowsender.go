// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// noopRingBuffer is a SenderManager that returns shadowSenders and discards all other operations.
type noopRingBuffer struct{}

func (n *noopRingBuffer) GetSender(_ checkid.ID) (sender.Sender, error) {
	return &shadowSender{}, nil
}

func (n *noopRingBuffer) SetSender(sender.Sender, checkid.ID) error {
	return nil
}

func (n *noopRingBuffer) DestroySender(_ checkid.ID) {}

func (n *noopRingBuffer) GetDefaultSender() (sender.Sender, error) {
	return &shadowSender{}, nil
}

// shadowSender implements sender.Sender and logs every data point instead of forwarding it.
type shadowSender struct{}

func (s *shadowSender) Commit() {
	log.Debugf("shadow sender: Commit")
}

func (s *shadowSender) Gauge(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: Gauge metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: GaugeNoIndex metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) Rate(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: Rate metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) Count(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: Count metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: MonotonicCount metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	log.Debugf("shadow sender: MonotonicCountWithFlushFirstValue metric=%s value=%v hostname=%s tags=%v flushFirstValue=%v", metric, value, hostname, tags, flushFirstValue)
}

func (s *shadowSender) Counter(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: Counter metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) Histogram(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: Histogram metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) Historate(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: Historate metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) Distribution(metric string, value float64, hostname string, tags []string) {
	log.Debugf("shadow sender: Distribution metric=%s value=%v hostname=%s tags=%v", metric, value, hostname, tags)
}

func (s *shadowSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
	log.Debugf("shadow sender: ServiceCheck checkName=%s status=%v hostname=%s tags=%v message=%s", checkName, status, hostname, tags, message)
}

func (s *shadowSender) OpenmetricsBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	log.Debugf("shadow sender: OpenmetricsBucket metric=%s value=%v [%v,%v) monotonic=%v hostname=%s tags=%v flushFirstValue=%v", metric, value, lowerBound, upperBound, monotonic, hostname, tags, flushFirstValue)
}

func (s *shadowSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	log.Debugf("shadow sender: HistogramBucket metric=%s value=%v [%v,%v) monotonic=%v hostname=%s tags=%v flushFirstValue=%v", metric, value, lowerBound, upperBound, monotonic, hostname, tags, flushFirstValue)
}

func (s *shadowSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	log.Debugf("shadow sender: GaugeWithTimestamp metric=%s value=%v hostname=%s tags=%v timestamp=%v", metric, value, hostname, tags, timestamp)
	return nil
}

func (s *shadowSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	log.Debugf("shadow sender: CountWithTimestamp metric=%s value=%v hostname=%s tags=%v timestamp=%v", metric, value, hostname, tags, timestamp)
	return nil
}

func (s *shadowSender) Event(e event.Event) {
	log.Debugf("shadow sender: Event title=%s host=%s", e.Title, e.Host)
}

func (s *shadowSender) EventPlatformEvent(rawEvent []byte, eventType string) {
	log.Debugf("shadow sender: EventPlatformEvent eventType=%s len=%d", eventType, len(rawEvent))
}

func (s *shadowSender) GetSenderStats() stats.SenderStats {
	return stats.SenderStats{}
}

func (s *shadowSender) DisableDefaultHostname(_ bool)   {}
func (s *shadowSender) SetCheckCustomTags(_ []string)   {}
func (s *shadowSender) SetCheckService(_ string)        {}
func (s *shadowSender) SetNoIndex(_ bool)               {}
func (s *shadowSender) FinalizeCheckServiceTag()        {}

func (s *shadowSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
	log.Debugf("shadow sender: OrchestratorMetadata clusterID=%s nodeType=%d msgs=%d", clusterID, nodeType, len(msgs))
}

func (s *shadowSender) OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string) {
	log.Debugf("shadow sender: OrchestratorManifest clusterID=%s msgs=%d", clusterID, len(msgs))
}
