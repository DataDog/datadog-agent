// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// RawSender interface to submit samples to aggregator directly
type RawSender interface {
	SendRawMetricSample(sample *metrics.MetricSample)
	SendRawServiceCheck(sc *servicecheck.ServiceCheck)
	Event(e event.Event)
}

// checkSender implements Sender
type checkSender struct {
	id                      checkid.ID
	defaultHostname         string
	defaultHostnameDisabled bool
	metricStats             stats.SenderStats
	priormetricStats        stats.SenderStats
	statsLock               sync.RWMutex
	itemsOut                chan<- senderItem
	serviceCheckOut         chan<- servicecheck.ServiceCheck
	eventOut                chan<- event.Event
	orchestratorMetadataOut chan<- senderOrchestratorMetadata
	orchestratorManifestOut chan<- senderOrchestratorManifest
	eventPlatformOut        chan<- senderEventPlatformEvent
	checkTags               []string
	service                 string
	noIndex                 bool
}

// senderItem knows how the aggregator should handle it
type senderItem interface {
	handle(agg *BufferedAggregator)
}

type senderMetricSample struct {
	id           checkid.ID
	metricSample *metrics.MetricSample
	commit       bool
}

func (s *senderMetricSample) handle(agg *BufferedAggregator) {
	panic("not called")
}

type senderHistogramBucket struct {
	id     checkid.ID
	bucket *metrics.HistogramBucket
}

func (s *senderHistogramBucket) handle(agg *BufferedAggregator) {
	panic("not called")
}

type senderEventPlatformEvent struct {
	id        checkid.ID
	rawEvent  []byte
	eventType string
}

type senderOrchestratorMetadata struct {
	msgs        []types.ProcessMessageBody
	clusterID   string
	payloadType int
}

type senderOrchestratorManifest struct {
	msgs      []types.ProcessMessageBody
	clusterID string
}

type checkSenderPool struct {
	agg     *BufferedAggregator
	senders map[checkid.ID]sender.Sender
	m       sync.Mutex
}

func newCheckSender(
	id checkid.ID,
	defaultHostname string,
	itemsOut chan<- senderItem,
	serviceCheckOut chan<- servicecheck.ServiceCheck,
	eventOut chan<- event.Event,
	orchestratorMetadataOut chan<- senderOrchestratorMetadata,
	orchestratorManifestOut chan<- senderOrchestratorManifest,
	eventPlatformOut chan<- senderEventPlatformEvent,
) *checkSender {
	panic("not called")
}

// DisableDefaultHostname allows check to override the default hostname that will be injected
// when no hostname is specified at submission (for metrics, events and service checks).
func (s *checkSender) DisableDefaultHostname(disable bool) {
	panic("not called")
}

// SetCheckCustomTags stores the tags set in the check configuration file.
// They will be appended to each send (metric, event and service)
func (s *checkSender) SetCheckCustomTags(tags []string) {
	panic("not called")
}

// SetCheckService appends the service as a tag for metrics, events, and service checks
// This may be called any number of times, though the only the last call will have an effect
func (s *checkSender) SetCheckService(service string) {
	panic("not called")
}

// FinalizeCheckServiceTag appends the service as a tag for metrics, events, and service checks
func (s *checkSender) FinalizeCheckServiceTag() {
	panic("not called")
}

func (s *checkSender) SetNoIndex(noIndex bool) {
	panic("not called")
}

// Commit commits the metric samples & histogram buckets that were added during a check run
// Should be called at the end of every check run
func (s *checkSender) Commit() {
	panic("not called")
}

func (s *checkSender) GetSenderStats() (metricStats stats.SenderStats) {
	panic("not called")
}

func (s *checkSender) cyclemetricStats() {
	panic("not called")
}

// SendRawMetricSample sends the raw sample
// Useful for testing - submitting precomputed samples.
func (s *checkSender) SendRawMetricSample(sample *metrics.MetricSample) {
	panic("not called")
}

func (s *checkSender) sendMetricSample(
	metric string,
	value float64,
	hostname string,
	tags []string,
	mType metrics.MetricType,
	flushFirstValue bool,
	noIndex bool) {
	panic("not called")
}

// Gauge should be used to send a simple gauge value to the aggregator. Only the last value sampled is kept at commit time.
func (s *checkSender) Gauge(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// GaugeNoIndex should be used to send a simple gauge value to the aggregator. Only the last value sampled is kept at commit time.
// This value is not indexed by the backend.
func (s *checkSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// Rate should be used to track the rate of a metric over each check run
func (s *checkSender) Rate(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// Count should be used to count a number of events that occurred during the check run
func (s *checkSender) Count(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// MonotonicCount should be used to track the increase of a monotonic raw counter
func (s *checkSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// MonotonicCountWithFlushFirstValue should be used to track the increase of a monotonic raw counter,
// and allows specifying whether the aggregator should flush the first sampled value as-is.
func (s *checkSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	panic("not called")
}

// Counter is DEPRECATED and only implemented to preserve backward compatibility with python checks. Prefer using either:
// * `Gauge` if you're counting states
// * `Count` if you're counting events
func (s *checkSender) Counter(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// Histogram should be used to track the statistical distribution of a set of values during a check run
// Should be called multiple times on the same (metric, hostname, tags) so that a distribution can be computed
func (s *checkSender) Histogram(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// HistogramBucket should be called to directly send raw buckets to be submitted as distribution metrics
func (s *checkSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	panic("not called")
}

// Historate should be used to create a histogram metric for "rate" like metrics.
// Warning this doesn't use the harmonic mean, beware of what it means when using it.
func (s *checkSender) Historate(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

func (s *checkSender) Distribution(metric string, value float64, hostname string, tags []string) {
	panic("not called")
}

// SendRawServiceCheck sends the raw service check
// Useful for testing - submitting precomputed service check.
func (s *checkSender) SendRawServiceCheck(sc *servicecheck.ServiceCheck) {
	panic("not called")
}

// ServiceCheck submits a service check
func (s *checkSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
	panic("not called")
}

// Event submits an event
func (s *checkSender) Event(e event.Event) {
	panic("not called")
}

// Event submits an event
func (s *checkSender) EventPlatformEvent(rawEvent []byte, eventType string) {
	panic("not called")
}

// OrchestratorMetadata submit orchestrator metadata messages
func (s *checkSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
	panic("not called")
}

func (s *checkSender) OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string) {
	panic("not called")
}

func (sp *checkSenderPool) getSender(id checkid.ID) (sender.Sender, error) {
	panic("not called")
}

func (sp *checkSenderPool) mkSender(id checkid.ID) (sender.Sender, error) {
	panic("not called")
}

func (sp *checkSenderPool) setSender(sender sender.Sender, id checkid.ID) error {
	panic("not called")
}

func (sp *checkSenderPool) removeSender(id checkid.ID) {
	panic("not called")
}
