// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/zstd"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
)

// HTTP client mock
type clientMock struct {
	body []byte
}

func (c *clientMock) Do(req *http.Request) (*http.Response, error) {
	c.body, _ = io.ReadAll(req.Body)
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
	}, nil
}

func newClientMock() client {
	return &clientMock{}
}

// Sender mock
type senderMock struct {
	sentMetrics []*agentmetric

	// Captures from the errortracking flush path. Protected by mu
	// because the flush job may run concurrently with test setup and
	// assertions; readers MUST take the lock or use a synchronisation
	// barrier (e.g. wait on runner.stop().Done) that establishes
	// happens-before with the job's completion.
	//
	// sendLogsCallCount counts sendLogsBatch invocations; sentLogs
	// flattens every batch into one accumulating slice. The pair lets
	// tests distinguish "1 call with N records" from "N calls with 1
	// record each" — the latter would be a regression to per-batch
	// dispatch that the flattened slice alone cannot detect.
	sentLogsMu        sync.Mutex
	sentLogs          []Log
	sendLogsCallCount int
}

func (s *senderMock) startSession(_ context.Context) *senderSession {
	return &senderSession{}
}
func (s *senderMock) flushSession(_ *senderSession) error {
	return nil
}
func (s *senderMock) sendAgentMetricPayloads(_ *senderSession, metrics []*agentmetric) {
	s.sentMetrics = append(s.sentMetrics, metrics...)
}
func (s *senderMock) sendEventPayload(_ *senderSession, _ *Event, _ map[string]interface{}) {
}
func (s *senderMock) sendLogsBatch(_ context.Context, logs []Log) error {
	s.sentLogsMu.Lock()
	defer s.sentLogsMu.Unlock()
	s.sendLogsCallCount++
	s.sentLogs = append(s.sentLogs, logs...)
	return nil
}

// capturedLogs returns a thread-safe snapshot of the records captured
// via sendLogsBatch. Tests should call this rather than reading
// sentLogs directly.
func (s *senderMock) capturedLogs() []Log {
	s.sentLogsMu.Lock()
	defer s.sentLogsMu.Unlock()
	out := make([]Log, len(s.sentLogs))
	copy(out, s.sentLogs)
	return out
}

// sendLogsCalls returns a thread-safe snapshot of how many times
// sendLogsBatch was invoked. Pair with capturedLogs to assert
// "one HTTP call per flush" (N records via 1 call, not 1 record via N
// calls).
func (s *senderMock) sendLogsCalls() int {
	s.sentLogsMu.Lock()
	defer s.sentLogsMu.Unlock()
	return s.sendLogsCallCount
}

// Runner mock (TODO: use use mock.Mock)
type runnerMock struct {
	mock.Mock
	jobs []job
}

func (r *runnerMock) run() {
	for _, j := range r.jobs {
		j.Run()
	}
}

func (r *runnerMock) start() {
}

func (r *runnerMock) stop() context.Context {
	return context.Background()
}

func (r *runnerMock) addJob(j job) {
	r.jobs = append(r.jobs, j)
}

func newRunnerMock() runner {
	return &runnerMock{}
}

// ------------------------------
// Utility functions

func makeStableMetricMap(metrics []*dto.Metric) map[string]*dto.Metric {
	if len(metrics) == 0 {
		return nil
	}

	metricMap := make(map[string]*dto.Metric)
	for _, m := range metrics {
		var tagsKeyBuilder strings.Builder

		// sort by names and values before insertion
		origTags := m.GetLabel()
		if len(origTags) > 0 {
			for _, tag := range cloneLabelsSorted(origTags) {
				tagsKeyBuilder.WriteString(tag.GetName())
				tagsKeyBuilder.WriteByte(':')
				tagsKeyBuilder.WriteString(tag.GetValue())
				tagsKeyBuilder.WriteByte(':')
			}
		}

		metricMap[tagsKeyBuilder.String()] = m
	}

	return metricMap
}

func metricLabelStrings(m *dto.Metric) []string {
	labels := make([]string, 0, len(m.GetLabel()))
	for _, label := range m.GetLabel() {
		labels = append(labels, label.GetName()+"="+label.GetValue())
	}
	return labels
}

func newLabelPair(name, value string) *dto.LabelPair {
	return &dto.LabelPair{Name: &name, Value: &value}
}

func newCounterMetric(value float64, labels ...*dto.LabelPair) *dto.Metric {
	return &dto.Metric{Label: labels, Counter: &dto.Counter{Value: &value}}
}

func newGaugeMetric(value float64, labels ...*dto.LabelPair) *dto.Metric {
	return &dto.Metric{Label: labels, Gauge: &dto.Gauge{Value: &value}}
}

func newHistogramMetricWithBucket(sampleCount, cumulativeCount uint64, labels ...*dto.LabelPair) *dto.Metric {
	upperBound := 10.0
	exemplarName := "trace_id"
	exemplarLabel := "abc123"
	exemplarValue := 1.5

	return &dto.Metric{
		Label: labels,
		Histogram: &dto.Histogram{
			SampleCount: &sampleCount,
			Bucket: []*dto.Bucket{
				{
					CumulativeCount: &cumulativeCount,
					UpperBound:      &upperBound,
					Exemplar: &dto.Exemplar{
						Label:     []*dto.LabelPair{newLabelPair(exemplarName, exemplarLabel)},
						Value:     &exemplarValue,
						Timestamp: timestamppb.New(time.Unix(100, 123)),
					},
				},
			},
		},
	}
}

func makeTelMock(t *testing.T) telemetry.Component {
	// Little hack. Telemetry component is not fully componentized, and relies on global registry so far
	// so we need to reset it before running the test. This is not ideal and will be improved in the future.
	tel := fxutil.Test[telemetry.Mock](t, mocktelemetry.Module())
	tel.Reset()
	return tel
}

func makeLogMock(t *testing.T) log.Component {
	return logmock.New(t)
}

func makeSenderImpl(t *testing.T, cl client, c string) sender {
	cfg := configmock.NewFromYAML(t, c)
	log := makeLogMock(t)
	if cl == nil {
		cl = newClientMock()
	}
	sndr, err := newSenderImpl(cfg, log, cl)
	assert.NoError(t, err)
	return sndr
}

// aggregator mock function
func getTestAtel(t *testing.T,
	tel telemetry.Component,
	YAMLConf string,
	sndr sender,
	client client,
	runner runner) *atel {

	if tel == nil {
		tel = makeTelMock(t)
	}

	if client == nil {
		client = newClientMock()
	}

	if runner == nil {
		runner = newRunnerMock()
	}

	cfg := configmock.NewFromYAML(t, YAMLConf)
	log := makeLogMock(t)

	var err error
	if sndr == nil {
		sndr, err = newSenderImpl(cfg, log, client)
	}
	assert.NoError(t, err)

	atel := createAtel(cfg, log, tel, sndr, runner, "agent")
	if atel == nil {
		err = errors.New("failed to create atel")
	}
	assert.NoError(t, err)

	return atel
}

func getCommonYAMLConfig(enabled bool, site string) string {
	if site == "" {
		return fmt.Sprintf("agent_telemetry:\n  enabled: %t", enabled)
	}
	return fmt.Sprintf("site: %s\nagent_telemetry:\n  enabled: %t", site, enabled)
}

func findErrortrackingJob(t *testing.T, runner *runnerMock) job {
	t.Helper()

	for _, scheduledJob := range runner.jobs {
		if scheduledJob.profiles == nil {
			return scheduledJob
		}
	}

	t.Fatal("errortracking job not found")
	return job{}
}

func TestCreateAtel_NegativeErrortrackingBufferSizeDoesNotPanic(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
site: datadoghq.com
agent_telemetry:
  enabled: true
  errortracking:
    enabled: true
    buffer_size: -1
`)
	log := makeLogMock(t)

	var atel *atel
	assert.NotPanics(t, func() {
		atel = createAtel(cfg, log, makeTelMock(t), &senderMock{}, &runnerMock{}, "agent")
	})
	require.NotNil(t, atel)
	require.NotNil(t, atel.errLogsCh)
	assert.Equal(t, defaultErrortrackingBufferSize, cap(atel.errLogsCh))
}

func TestCreateAtel_NegativeErrortrackingValuesFallbackToSafeDefaults(t *testing.T) {
	runner := &runnerMock{}
	atel := getTestAtel(t, nil, `
site: datadoghq.com
agent_telemetry:
  enabled: true
  errortracking:
    enabled: true
    flush_interval_seconds: -1
    startup_jitter_seconds: -1
    shutdown_drain_timeout_seconds: -1
`, &senderMock{}, nil, runner)

	assert.Equal(t, 60*time.Second, atel.errLogsFlushInterval)
	assert.Equal(t, time.Duration(0), atel.errLogsStartupJitter)
	assert.Equal(t, 5*time.Second, atel.shutdownDrainTimeout)

	assert.NotPanics(t, func() {
		require.NoError(t, atel.start())
	})
	t.Cleanup(func() {
		atel.cancel()
	})

	errortrackingJob := findErrortrackingJob(t, runner)
	assert.Equal(t, uint(defaultErrortrackingFlushIntervalSeconds), errortrackingJob.schedule.Period)
	assert.Equal(t, uint(0), errortrackingJob.schedule.StartAfter)
}

func TestCreateAtel_ErrortrackingZeroValuesPreserveCurrentSemantics(t *testing.T) {
	runner := &runnerMock{}
	atel := getTestAtel(t, nil, `
site: datadoghq.com
agent_telemetry:
  enabled: true
  errortracking:
    enabled: true
    buffer_size: 0
    flush_interval_seconds: 0
    startup_jitter_seconds: 0
    shutdown_drain_timeout_seconds: 0
`, &senderMock{}, nil, runner)

	require.NotNil(t, atel.errLogsCh)
	assert.Equal(t, 0, cap(atel.errLogsCh))
	assert.Equal(t, time.Duration(0), atel.errLogsFlushInterval)
	assert.Equal(t, time.Duration(0), atel.errLogsStartupJitter)
	assert.Equal(t, time.Duration(0), atel.shutdownDrainTimeout)

	require.NoError(t, atel.start())
	t.Cleanup(func() {
		atel.cancel()
	})

	errortrackingJob := findErrortrackingJob(t, runner)
	assert.Equal(t, uint(5), errortrackingJob.schedule.Period)
	assert.Equal(t, uint(0), errortrackingJob.schedule.StartAfter)
}

func (p *Payload) UnmarshalAgentMetrics(itfPayload map[string]interface{}) error {
	var ok bool

	p.RequestType = "agent-metrics"
	p.APIVersion = itfPayload["request_type"].(string)

	var metricsItfPayload map[string]interface{}
	metricsItfPayload, ok = itfPayload["payload"].(map[string]interface{})
	if !ok {
		return errors.New("payload not found")
	}
	var metricsItf map[string]interface{}
	metricsItf, ok = metricsItfPayload["metrics"].(map[string]interface{})
	if !ok {
		return errors.New("metrics not found")
	}

	var err error
	var metricsPayload AgentMetricsPayload
	metricsPayload.Metrics = make(map[string]interface{})
	for k, v := range metricsItf {
		if k == "agent_metadata" {
			// Re(un)marshal the meatadata
			var metadata AgentMetadataPayload
			var metadataBytes []byte
			if metadataBytes, err = json.Marshal(v); err != nil {
				return err
			}
			if err = json.Unmarshal(metadataBytes, &metadata); err != nil {
				return err
			}
			metricsPayload.Metrics[k] = metadata
		} else {
			// Re(un)marshal the metric
			var metric MetricPayload
			var metricBytes []byte
			if metricBytes, err = json.Marshal(v); err != nil {
				return err
			}
			if err = json.Unmarshal(metricBytes, &metric); err != nil {
				return err
			}
			metricsPayload.Metrics[k] = metric
		}
	}
	p.Payload = metricsPayload
	return nil
}

func (p *Payload) UnmarshalMessageBatch(itfPayload map[string]interface{}) error {
	payloadsRaw, ok := itfPayload["payload"].([]interface{})
	if !ok {
		return errors.New("payload not found")
	}

	// ensure all payloads which should be agent-metrics
	var payloads []Payload
	for _, payloadRaw := range payloadsRaw {
		itfChildPayload, ok := payloadRaw.(map[string]interface{})
		if !ok {
			return errors.New("invalid payload item type")
		}

		requestTypeRaw, ok := itfChildPayload["request_type"]
		if !ok {
			return errors.New("request_type not found")
		}
		requestType, ok := requestTypeRaw.(string)
		if !ok {
			return errors.New("request_type type is invalid")
		}

		if requestType != "agent-metrics" {
			return errors.New("request_type should be agent-metrics")
		}

		var payload Payload
		if err := payload.UnmarshalAgentMetrics(itfChildPayload); err != nil {
			return err
		}
		payloads = append(payloads, payload)

	}
	p.Payload = payloads

	return nil
}

// This is a unit test function do not use it for actual code (at least yet)
// since it is not 100% full implementation of the unmarshalling
func (p *Payload) UnmarshalJSON(b []byte) (err error) {
	var itfPayload map[string]interface{}
	if err := json.Unmarshal(b, &itfPayload); err != nil {
		return err
	}

	requestTypeRaw, ok := itfPayload["request_type"]
	if !ok {
		return errors.New("request_type not found")
	}
	requestType, ok := requestTypeRaw.(string)
	if !ok {
		return errors.New("request_type type is invalid")
	}

	if requestType == "agent-metrics" {
		return p.UnmarshalAgentMetrics(itfPayload)
	}

	if requestType == "message-batch" {
		return p.UnmarshalMessageBatch(itfPayload)
	}

	return errors.New("request_type should be either agent-metrics or message-batch")
}

func getPayload(a *atel) (*Payload, error) {
	payloadJSON, err := a.getAsJSON()
	if err != nil {
		return nil, err
	}

	var payload Payload
	err = json.Unmarshal(payloadJSON, &payload)
	return &payload, err
}

func getPayloadMetric(a *atel, metricName string) (*MetricPayload, bool) {
	payload, err := getPayload(a)
	if err != nil {
		return nil, false
	}
	metrics := payload.Payload.(AgentMetricsPayload).Metrics
	if metricItf, ok := metrics[metricName]; ok {
		metric := metricItf.(MetricPayload)
		return &metric, true
	}

	return nil, false
}

// If you have multiple metrics with the same name (timeseries) and filtered by a metric, use getPayloadFilteredMetricList
func getPayloadFilteredMetricList(a *atel, metricName string) ([]*MetricPayload, bool) {
	payload, err := getPayload(a)
	if err != nil {
		return nil, false
	}

	var payloads []*MetricPayload
	for _, payload := range payload.Payload.([]Payload) {
		metrics := payload.Payload.(AgentMetricsPayload).Metrics
		if metricItf, ok := metrics[metricName]; ok {
			metric := metricItf.(MetricPayload)
			payloads = append(payloads, &metric)
		}
	}

	return payloads, true
}

// getPayloadMetricMap returns metrics by name when each name has only one emitted time series.
func getPayloadMetricMap(a *atel) map[string]*MetricPayload {
	payload, err := getPayload(a)
	if err != nil {
		return nil
	}

	payloads := make(map[string]*MetricPayload)

	if mm, ok := payload.Payload.([]Payload); ok {
		for _, payload := range mm {
			metrics := payload.Payload.(AgentMetricsPayload).Metrics
			for metricName, metricItf := range metrics {
				metric := metricItf.(MetricPayload)
				payloads[metricName] = &metric
			}
		}
		return payloads
	}

	if m, ok := payload.Payload.(AgentMetricsPayload); ok {
		metrics := m.Metrics
		for metricName, metricItf := range metrics {
			if metric, ok2 := metricItf.(MetricPayload); ok2 {
				payloads[metricName] = &metric
			}
		}
		return payloads
	}

	return nil
}

func getPayloadMetricByTagValues(metrics []*MetricPayload, tags map[string]interface{}) (*MetricPayload, bool) {
	for _, m := range metrics {
		if maps.Equal(m.Tags, tags) {
			return m, true
		}
	}

	return nil, false
}

// Validate the payload

// metric, ok := metrics["foo.bar"]

// ------------------------------
// Tests

func TestLocalEmitterNormalization(t *testing.T) {
	tests := []struct {
		flavor string
		want   string
	}{
		{flavor: "agent", want: "agent"},
		{flavor: "cluster_agent", want: "cluster-agent"},
		{flavor: "trace_agent", want: "trace-agent"},
		{flavor: "otel_agent", want: "otel-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.flavor, func(t *testing.T) {
			assert.Equal(t, tt.want, localEmitterFromFlavor(tt.flavor))
		})
	}
}

func TestLocalEmitterCreateAtelStoresNormalizedFlavor(t *testing.T) {
	cfg := configmock.NewFromYAML(t, getCommonYAMLConfig(true, "foo.bar"))
	a := createAtel(
		cfg,
		makeLogMock(t),
		makeTelMock(t),
		&senderMock{},
		&runnerMock{},
		localEmitterFromFlavor("cluster_agent"),
	)

	require.True(t, a.enabled)
	assert.Equal(t, "cluster-agent", a.localEmitter)
}

func TestEnabled(t *testing.T) {
	a := getTestAtel(t, nil, getCommonYAMLConfig(true, "foo.bar"), nil, nil, nil)
	assert.True(t, a.enabled)
}

func TestDisable(t *testing.T) {
	a := getTestAtel(t, nil, getCommonYAMLConfig(false, "foo.bar"), nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestDisableIfFipsEnabled(t *testing.T) {
	c := `
site: "foo.bar"
agent_telemetry:
  enabled: true
fips:
  enabled: true
`
	a := getTestAtel(t, nil, c, nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestEnableIfFipsDisabled(t *testing.T) {
	c := `
site: "foo.bar"
agent_telemetry:
  enabled: true
fips:
  enabled: false
`
	a := getTestAtel(t, nil, c, nil, nil, nil)
	assert.True(t, a.enabled)
}

func TestDisableIfGovCloud(t *testing.T) {
	c := `
site: "ddog-gov.com"
agent_telemetry:
  enabled: true
`
	a := getTestAtel(t, nil, c, nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestDisableIfLongGovCloud(t *testing.T) {
	c := `
site: "xxxx99.ddog-gov.com"
agent_telemetry:
  enabled: true
`
	a := getTestAtel(t, nil, c, nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestEnableIfNotGovCloud(t *testing.T) {
	c := `
site: "datadoghq.eu"
agent_telemetry:
  enabled: true
`
	a := getTestAtel(t, nil, c, nil, nil, nil)
	assert.True(t, a.enabled)
}

func TestRun(t *testing.T) {
	r := newRunnerMock()
	a := getTestAtel(t, nil, getCommonYAMLConfig(true, "foo.bar"), nil, nil, r)
	assert.True(t, a.enabled)

	a.start()

	// Default configuration has 6 jobs with different schedules:
	fmt.Println(r.(*runnerMock).jobs)
	assert.Equal(t, 6, len(r.(*runnerMock).jobs))

	// Verify we have the expected number of profiles across all jobs
	totalProfiles := 0
	for _, job := range r.(*runnerMock).jobs {
		totalProfiles += len(job.profiles)
	}
	fmt.Println(totalProfiles)
	// Default config has 20 profiles total (checks, logs-and-metrics, database, synthetics, connectivity, csi-driver, agent-performance, service-discovery, runtime-started, runtime-running, hostname, rtloader, otlp, procmgr, trace-agent, gpu, cluster-agent, injector, ebpf, autodiscovery-discovery-probe)
	assert.Equal(t, 20, totalProfiles)
}

func TestReportMetricBasic(t *testing.T) {
	tel := makeTelMock(t)
	counter := tel.NewCounter("checks", "execution_time", []string{"check_name"}, "")
	counter.Inc("mycheck")

	c := newClientMock()
	r := newRunnerMock()
	a := getTestAtel(t, tel, getCommonYAMLConfig(true, "foo.bar"), nil, c, r)
	require.True(t, a.enabled)

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	assert.True(t, len(c.(*clientMock).body) > 0)
}

func TestNoUserLabelSpecifiedAggregationCounter(t *testing.T) {
	c := `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                preserve_tags: []
  `

	// setup and initiate atel
	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "zoo", []string{"tag1", "tag2", "tag3"}, "")
	counter.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})
	counter.AddWithTags(20, map[string]string{"tag1": "a2", "tag2": "b2", "tag3": "c2"})
	counter.AddWithTags(30, map[string]string{"tag1": "a3", "tag2": "b3", "tag3": "c3"})

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 1 metric sent
	assert.Equal(t, 1, len(s.sentMetrics))

	// aggregated to 10 + 20 + 30 = 60
	m := s.sentMetrics[0].metrics[0]
	assert.Equal(t, float64(60), m.Counter.GetValue())
	assert.Equal(t, []string{"emitter=agent"}, metricLabelStrings(m))
}

func TestNoUserLabelSpecifiedExplicitAggregationGauge(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                preserve_tags: []
  `

	// setup and initiate atel
	tel := makeTelMock(t)
	gauge := tel.NewGauge("bar", "zoo", []string{"tag1", "tag2", "tag3"}, "")
	gauge.WithTags(map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"}).Set(10)
	gauge.WithTags(map[string]string{"tag1": "a2", "tag2": "b2", "tag3": "c2"}).Set(20)
	gauge.WithTags(map[string]string{"tag1": "a3", "tag2": "b3", "tag3": "c3"}).Set(30)

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 1 metric sent
	assert.Equal(t, 1, len(s.sentMetrics))

	// aggregated to 10 + 20 + 30 = 60
	m := s.sentMetrics[0].metrics[0]
	assert.Equal(t, float64(60), m.Gauge.GetValue())
	assert.Equal(t, []string{"emitter=agent"}, metricLabelStrings(m))
}

func TestNoUserLabelSpecifiedImplicitAggregationGauge(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
  `

	// setup and initiate atel
	tel := makeTelMock(t)
	gauge := tel.NewGauge("bar", "zoo", []string{"tag1", "tag2", "tag3"}, "")
	gauge.WithTags(map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"}).Set(10)
	gauge.WithTags(map[string]string{"tag1": "a2", "tag2": "b2", "tag3": "c2"}).Set(20)
	gauge.WithTags(map[string]string{"tag1": "a3", "tag2": "b3", "tag3": "c3"}).Set(30)

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 1 metric sent
	assert.Equal(t, 1, len(s.sentMetrics))

	// aggregated to 10 + 20 + 30 = 60
	m := s.sentMetrics[0].metrics[0]
	assert.Equal(t, float64(60), m.Gauge.GetValue())
	assert.Equal(t, []string{"emitter=agent"}, metricLabelStrings(m))
}

func TestNoUserLabelSpecifiedAggregationHistogram(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                preserve_tags: []
  `

	// setup and initiate atel
	tel := makeTelMock(t)
	buckets := []float64{10, 100, 1000, 10000}
	hist := tel.NewHistogram("bar", "zoo", []string{"tag1", "tag2", "tag3"}, "", buckets)
	hist.WithTags(map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"}).Observe(1001)
	hist.WithTags(map[string]string{"tag1": "a2", "tag2": "b2", "tag3": "c2"}).Observe(1002)
	hist.WithTags(map[string]string{"tag1": "a3", "tag2": "b3", "tag3": "c3"}).Observe(1003)

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 1 metric sent
	require.Equal(t, 1, len(s.sentMetrics))
	require.True(t, len(s.sentMetrics[0].metrics) > 0)

	// aggregated to 10 + 20 + 30 = 60
	m := s.sentMetrics[0].metrics[0]
	assert.Equal(t, uint64(3), m.Histogram.GetBucket()[3].GetCumulativeCount())
	assert.Equal(t, []string{"emitter=agent"}, metricLabelStrings(m))
}

// TestAggregateTagsAliasBackwardCompat verifies that the deprecated aggregate_tags YAML key
// is accepted and behaves identically to preserve_tags for existing custom configurations.
func TestAggregateTagsAliasBackwardCompat(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                aggregate_tags:
                  - tag1
    `
	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "zoo", []string{"tag1", "tag2"}, "")
	counter.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1"})
	counter.AddWithTags(20, map[string]string{"tag1": "a1", "tag2": "b2"})
	counter.AddWithTags(30, map[string]string{"tag1": "a2", "tag2": "b3"})

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	a.start()
	r.(*runnerMock).run()

	// aggregate_tags: [tag1] should aggregate by emitter and tag1, dropping tag2.
	require.Equal(t, 1, len(s.sentMetrics))
	require.Equal(t, 2, len(s.sentMetrics[0].metrics))
	metrics := makeStableMetricMap(s.sentMetrics[0].metrics)

	require.Contains(t, metrics, "emitter:agent:tag1:a1:")
	assert.Equal(t, float64(30), metrics["emitter:agent:tag1:a1:"].Counter.GetValue())
	require.Contains(t, metrics, "emitter:agent:tag1:a2:")
	assert.Equal(t, float64(30), metrics["emitter:agent:tag1:a2:"].Counter.GetValue())
}

func TestTagSpecifiedAggregationCounter(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                preserve_tags:
                  - tag1
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "zoo", []string{"tag1", "tag2", "tag3"}, "")

	// should generate 2 timeseries withj tag1:a1, tag1:a2
	counter.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})
	counter.AddWithTags(20, map[string]string{"tag1": "a1", "tag2": "b2", "tag3": "c2"})
	counter.AddWithTags(30, map[string]string{"tag1": "a2", "tag2": "b3", "tag3": "c3"})

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 2 metric should be sent
	require.Equal(t, 1, len(s.sentMetrics))
	require.Equal(t, 2, len(s.sentMetrics[0].metrics))

	// order is not deterministic, use label key to identify the metrics
	metrics := makeStableMetricMap(s.sentMetrics[0].metrics)

	// aggregated
	require.Contains(t, metrics, "emitter:agent:tag1:a1:")
	m1 := metrics["emitter:agent:tag1:a1:"]
	assert.Equal(t, float64(30), m1.Counter.GetValue())

	require.Contains(t, metrics, "emitter:agent:tag1:a2:")
	m2 := metrics["emitter:agent:tag1:a2:"]
	assert.Equal(t, float64(30), m2.Counter.GetValue())
}

func TestCounterDeltaCacheLabelKeyDoesNotCollideOnDelimiters(t *testing.T) {
	const config = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.counter
                preserve_tags:
                  - a
                  - c
    `

	tel := makeTelMock(t)
	a := getTestAtel(t, tel, config, makeSenderImpl(t, nil, config), nil, newRunnerMock())
	require.True(t, a.enabled)

	counter := tel.NewCounter("foo", "counter", []string{"a", "c"}, "")
	firstTags := map[string]string{"a": "x:c:y", "c": "z"}
	secondTags := map[string]string{"a": "x", "c": "y:c:z"}
	firstPayloadTags := map[string]interface{}{"a": "x:c:y", "c": "z", "emitter": "agent"}
	secondPayloadTags := map[string]interface{}{"a": "x", "c": "y:c:z", "emitter": "agent"}

	assertDeltas := func(firstExpected, secondExpected float64) {
		metrics, ok := getPayloadFilteredMetricList(a, "foo.counter")
		require.True(t, ok)
		require.Len(t, metrics, 2)

		first, ok := getPayloadMetricByTagValues(metrics, firstPayloadTags)
		require.True(t, ok)
		second, ok := getPayloadMetricByTagValues(metrics, secondPayloadTags)
		require.True(t, ok)
		assert.Equal(t, firstExpected, first.Value)
		assert.Equal(t, secondExpected, second.Value)
	}

	counter.AddWithTags(10, firstTags)
	counter.AddWithTags(100, secondTags)
	assertDeltas(10, 100)

	counter.AddWithTags(3, firstTags)
	counter.AddWithTags(7, secondTags)
	assertDeltas(3, 7)
}

func TestHistogramDeltaCacheLabelKeyDoesNotCollideOnDelimiters(t *testing.T) {
	const config = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.histogram
                preserve_tags:
                  - a
                  - c
    `

	tel := makeTelMock(t)
	a := getTestAtel(t, tel, config, makeSenderImpl(t, nil, config), nil, newRunnerMock())
	require.True(t, a.enabled)

	histogram := tel.NewHistogram("foo", "histogram", []string{"a", "c"}, "", []float64{1})
	firstTags := map[string]string{"a": "x:c:y", "c": "z"}
	secondTags := map[string]string{"a": "x", "c": "y:c:z"}
	firstPayloadTags := map[string]interface{}{"a": "x:c:y", "c": "z", "emitter": "agent"}
	secondPayloadTags := map[string]interface{}{"a": "x", "c": "y:c:z", "emitter": "agent"}

	observe := func(tags map[string]string, explicitBucketCount, infBucketCount int) {
		for range explicitBucketCount {
			histogram.WithTags(tags).Observe(0.5)
		}
		for range infBucketCount {
			histogram.WithTags(tags).Observe(2)
		}
	}
	assertDeltas := func(firstExpected, secondExpected map[string]uint64) {
		payload, err := getPayload(a)
		require.NoError(t, err)
		payloads, ok := payload.Payload.([]Payload)
		require.True(t, ok)
		metrics := make([]*MetricPayload, 0, len(payloads))
		for _, payload := range payloads {
			agentMetrics, ok := payload.Payload.(AgentMetricsPayload)
			require.True(t, ok)
			metricValue, ok := agentMetrics.Metrics["foo.histogram"]
			require.True(t, ok)
			metric, ok := metricValue.(MetricPayload)
			require.True(t, ok)
			metrics = append(metrics, &metric)
		}
		require.Len(t, metrics, 2)

		first, ok := getPayloadMetricByTagValues(metrics, firstPayloadTags)
		require.True(t, ok)
		second, ok := getPayloadMetricByTagValues(metrics, secondPayloadTags)
		require.True(t, ok)
		assert.Equal(t, firstExpected, first.Buckets)
		assert.Equal(t, secondExpected, second.Buckets)
	}

	observe(firstTags, 1, 1)
	observe(secondTags, 4, 2)
	assertDeltas(
		map[string]uint64{"1": 1, "+Inf": 1},
		map[string]uint64{"1": 4, "+Inf": 2},
	)

	observe(firstTags, 2, 1)
	observe(secondTags, 3, 2)
	assertDeltas(
		map[string]uint64{"1": 2, "+Inf": 1},
		map[string]uint64{"1": 3, "+Inf": 2},
	)
}

func TestTagAggregateTotalCounter(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                aggregate_total: true
                preserve_tags:
                  - tag1
    `
	// setup and initiate atel
	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "zoo", []string{"tag1", "tag2", "tag3"}, "")

	// should generate 4 timeseries withj tag1:a1, tag1:a2, tag1:a3 and total:6
	counter.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})
	counter.AddWithTags(20, map[string]string{"tag1": "a1", "tag2": "b2", "tag3": "c2"})
	counter.AddWithTags(30, map[string]string{"tag1": "a2", "tag2": "b3", "tag3": "c3"})
	counter.AddWithTags(40, map[string]string{"tag1": "a3", "tag2": "b4", "tag3": "c4"})
	counter.AddWithTags(50, map[string]string{"tag1": "a3", "tag2": "b5", "tag3": "c5"})
	counter.AddWithTags(60, map[string]string{"tag1": "a3", "tag2": "b6", "tag3": "c6"})

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 4 metric sent
	require.Equal(t, 1, len(s.sentMetrics))
	require.Equal(t, 4, len(s.sentMetrics[0].metrics))

	// order is not deterministic, use label key to identify the metrics
	metrics := makeStableMetricMap(s.sentMetrics[0].metrics)

	// aggregated
	require.Contains(t, metrics, "emitter:agent:tag1:a1:")
	m1 := metrics["emitter:agent:tag1:a1:"]
	assert.Equal(t, float64(30), m1.Counter.GetValue())

	require.Contains(t, metrics, "emitter:agent:tag1:a2:")
	m2 := metrics["emitter:agent:tag1:a2:"]
	assert.Equal(t, float64(30), m2.Counter.GetValue())

	require.Contains(t, metrics, "emitter:agent:tag1:a3:")
	m3 := metrics["emitter:agent:tag1:a3:"]
	assert.Equal(t, float64(150), m3.Counter.GetValue())

	require.Contains(t, metrics, "emitter:agent:total:6:")
	m4 := metrics["emitter:agent:total:6:"]
	assert.Equal(t, []string{"emitter=agent", "total=6"}, metricLabelStrings(m4))
	assert.Equal(t, float64(210), m4.Counter.GetValue())
}

func TestAggregateTotalCounterPerEmitter(t *testing.T) {
	mCfg := &MetricConfig{
		AggregateTotal:     true,
		preserveTagsExists: true,
		preserveTagsMap:    map[string]any{"tag1": struct{}{}},
	}
	metrics := []*dto.Metric{
		newCounterMetric(10, newLabelPair("tag1", "a1")),
		newCounterMetric(20, newLabelPair("emitter", ""), newLabelPair("tag1", "a2")),
		newCounterMetric(30, newLabelPair("emitter", "system-probe"), newLabelPair("tag1", "a1")),
	}

	results := (&atel{localEmitter: "agent"}).aggregateMetricTags(mCfg, dto.MetricType_COUNTER, metrics)

	require.Len(t, results, 5)
	metricsByTag := makeStableMetricMap(results)
	require.Contains(t, metricsByTag, "emitter:agent:tag1:a1:")
	require.Equal(t, 10.0, metricsByTag["emitter:agent:tag1:a1:"].Counter.GetValue())
	require.Contains(t, metricsByTag, "emitter:agent:tag1:a2:")
	require.Equal(t, 20.0, metricsByTag["emitter:agent:tag1:a2:"].Counter.GetValue())
	require.Contains(t, metricsByTag, "emitter:system-probe:tag1:a1:")
	require.Equal(t, 30.0, metricsByTag["emitter:system-probe:tag1:a1:"].Counter.GetValue())

	agentTotal := metricsByTag["emitter:agent:total:2:"]
	require.NotNil(t, agentTotal)
	require.Equal(t, []string{"emitter=agent", "total=2"}, metricLabelStrings(agentTotal))
	require.Equal(t, 30.0, agentTotal.Counter.GetValue())

	systemProbeTotal := metricsByTag["emitter:system-probe:total:1:"]
	require.NotNil(t, systemProbeTotal)
	require.Equal(t, []string{"emitter=system-probe", "total=1"}, metricLabelStrings(systemProbeTotal))
	require.Equal(t, 30.0, systemProbeTotal.Counter.GetValue())
}

func TestAggregateTotalEmitterNamedTotalKeepsOrdinaryAndTotalOutputs(t *testing.T) {
	mCfg := &MetricConfig{
		AggregateTotal:     true,
		preserveTagsExists: true,
		preserveTagsMap:    map[string]any{"tag1": struct{}{}},
	}
	metric := newCounterMetric(7, newLabelPair("emitter", "total"), newLabelPair("tag1", "a1"))

	results := (&atel{localEmitter: "agent"}).aggregateMetricTags(mCfg, dto.MetricType_COUNTER, []*dto.Metric{metric})

	require.Len(t, results, 2)
	metricsByTag := makeStableMetricMap(results)
	ordinary := metricsByTag["emitter:total:tag1:a1:"]
	require.NotNil(t, ordinary)
	require.Equal(t, 7.0, ordinary.Counter.GetValue())
	total := metricsByTag["emitter:total:total:1:"]
	require.NotNil(t, total)
	require.Equal(t, []string{"emitter=total", "total=1"}, metricLabelStrings(total))
	require.Equal(t, 7.0, total.Counter.GetValue())
}

func TestAggregateTotalHistogramPerEmitterHasIndependentOutputs(t *testing.T) {
	mCfg := &MetricConfig{
		AggregateTotal:     true,
		preserveTagsExists: true,
		preserveTagsMap:    map[string]any{"group": struct{}{}},
	}
	sources := []*dto.Metric{
		newHistogramMetricWithBucket(3, 2, newLabelPair("group", "a")),
		newHistogramMetricWithBucket(5, 4, newLabelPair("group", "b")),
		newHistogramMetricWithBucket(7, 6, newLabelPair("emitter", "system-probe"), newLabelPair("group", "a")),
	}
	sourceSnapshots := make([]*dto.Metric, len(sources))
	for i, source := range sources {
		sourceSnapshots[i] = proto.Clone(source).(*dto.Metric)
	}

	results := (&atel{localEmitter: "agent"}).aggregateMetricTags(mCfg, dto.MetricType_HISTOGRAM, sources)

	require.Len(t, results, 5)
	metricsByTag := makeStableMetricMap(results)
	agentGroupA := metricsByTag["emitter:agent:group:a:"]
	agentGroupB := metricsByTag["emitter:agent:group:b:"]
	systemProbeGroupA := metricsByTag["emitter:system-probe:group:a:"]
	agentTotal := metricsByTag["emitter:agent:total:2:"]
	systemProbeTotal := metricsByTag["emitter:system-probe:total:1:"]
	require.NotNil(t, agentGroupA)
	require.NotNil(t, agentGroupB)
	require.NotNil(t, systemProbeGroupA)
	require.NotNil(t, agentTotal)
	require.NotNil(t, systemProbeTotal)

	require.Equal(t, uint64(3), agentGroupA.Histogram.GetSampleCount())
	require.Equal(t, uint64(2), agentGroupA.Histogram.GetBucket()[0].GetCumulativeCount())
	require.Equal(t, uint64(5), agentGroupB.Histogram.GetSampleCount())
	require.Equal(t, uint64(4), agentGroupB.Histogram.GetBucket()[0].GetCumulativeCount())
	require.Equal(t, uint64(7), systemProbeGroupA.Histogram.GetSampleCount())
	require.Equal(t, uint64(6), systemProbeGroupA.Histogram.GetBucket()[0].GetCumulativeCount())
	require.Equal(t, []string{"emitter=agent", "total=2"}, metricLabelStrings(agentTotal))
	require.Equal(t, uint64(8), agentTotal.Histogram.GetSampleCount())
	require.Equal(t, uint64(6), agentTotal.Histogram.GetBucket()[0].GetCumulativeCount())
	require.Equal(t, []string{"emitter=system-probe", "total=1"}, metricLabelStrings(systemProbeTotal))
	require.Equal(t, uint64(7), systemProbeTotal.Histogram.GetSampleCount())
	require.Equal(t, uint64(6), systemProbeTotal.Histogram.GetBucket()[0].GetCumulativeCount())

	for i, source := range sources {
		require.True(t, proto.Equal(sourceSnapshots[i], source), "source histogram %d was mutated", i)
	}
	requireHistogramPointersIndependent(t, agentGroupA.Histogram, sources[0].Histogram)
	requireHistogramPointersIndependent(t, agentGroupB.Histogram, sources[1].Histogram)
	requireHistogramPointersIndependent(t, systemProbeGroupA.Histogram, sources[2].Histogram)
	requireHistogramPointersIndependent(t, agentTotal.Histogram, agentGroupA.Histogram)
	requireHistogramPointersIndependent(t, systemProbeTotal.Histogram, systemProbeGroupA.Histogram)

	agentGroupASnapshot := proto.Clone(agentGroupA).(*dto.Metric)
	*agentTotal.Histogram.SampleCount = 80
	*agentTotal.Histogram.Bucket[0].CumulativeCount = 60
	*agentTotal.Histogram.Bucket[0].UpperBound = 20
	*agentTotal.Histogram.Bucket[0].Exemplar.Value = 3.5
	agentTotal.Histogram.Bucket[0].Exemplar.Timestamp.Seconds = 300
	require.True(t, proto.Equal(agentGroupASnapshot, agentGroupA), "mutating total changed grouped histogram")
	for i, source := range sources {
		require.True(t, proto.Equal(sourceSnapshots[i], source), "mutating total changed source histogram %d", i)
	}
}

func TestCompileMetricEmitterPreserveTags(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		tags                   string
		wantPreserveTags       []string
		wantAggregateTags      []string
		wantCompiledTags       map[string]any
		wantPreserveTagsExists bool
	}{
		{
			name: "deprecated aggregate_tags emitter and user tag",
			tags: `
            aggregate_tags:
              - emitter
              - compression_kind`,
			wantAggregateTags:      []string{"emitter", "compression_kind"},
			wantCompiledTags:       map[string]any{"compression_kind": struct{}{}},
			wantPreserveTagsExists: true,
		},
		{
			name: "preserve_tags precedence retains both source fields",
			tags: `
            preserve_tags:
              - emitter
            aggregate_tags:
              - compression_kind`,
			wantPreserveTags:  []string{"emitter"},
			wantAggregateTags: []string{"compression_kind"},
			wantCompiledTags:  map[string]any{},
		},
		{
			name: "empty preserve_tags falls back and retains both source fields",
			tags: `
            preserve_tags: []
            aggregate_tags:
              - emitter
              - compression_kind`,
			wantPreserveTags:       []string{},
			wantAggregateTags:      []string{"emitter", "compression_kind"},
			wantCompiledTags:       map[string]any{"compression_kind": struct{}{}},
			wantPreserveTagsExists: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, fmt.Sprintf(`
agent_telemetry:
  enabled: true
  profiles:
    - name: foo
      metric:
        metrics:
          - name: bar.zoo%s
`, tt.tags))

			atelCfg, err := parseConfig(cfg)
			require.NoError(t, err)
			mCfg := &atelCfg.Profiles[0].Metric.Metrics[0]
			require.Equal(t, tt.wantPreserveTags, mCfg.PreserveTags)
			require.Equal(t, tt.wantAggregateTags, mCfg.AggregateTags)
			require.Equal(t, tt.wantCompiledTags, mCfg.preserveTagsMap)
			require.Equal(t, tt.wantPreserveTagsExists, mCfg.preserveTagsExists)
		})
	}
}

func TestAggregateTotalRejectsReservedTotalPreserveTag(t *testing.T) {
	const wantErr = "profile 'foo' metric 'bar.zoo' cannot preserve reserved tag 'total' when aggregate_total is enabled"

	for _, tt := range []struct {
		name                string
		aggregateTotal      bool
		tags                string
		wantErr             bool
		wantPreservedTags   []string
		wantUnpreservedTags []string
	}{
		{
			name:           "preserve_tags with aggregate total enabled",
			aggregateTotal: true,
			tags: `
            preserve_tags:
              - total`,
			wantErr: true,
		},
		{
			name: "preserve_tags with aggregate total disabled",
			tags: `
            preserve_tags:
              - total`,
			wantPreservedTags: []string{"total"},
		},
		{
			name:           "deprecated aggregate_tags with aggregate total enabled",
			aggregateTotal: true,
			tags: `
            aggregate_tags:
              - total`,
			wantErr: true,
		},
		{
			name:           "preserve_tags takes precedence over deprecated alias",
			aggregateTotal: true,
			tags: `
            preserve_tags:
              - tag1
            aggregate_tags:
              - total`,
			wantPreservedTags:   []string{"tag1"},
			wantUnpreservedTags: []string{"total"},
		},
		{
			name:           "empty preserve_tags falls back to deprecated alias",
			aggregateTotal: true,
			tags: `
            preserve_tags: []
            aggregate_tags:
              - total`,
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, fmt.Sprintf(`
agent_telemetry:
  enabled: true
  profiles:
    - name: foo
      metric:
        metrics:
          - name: bar.zoo
            aggregate_total: %t%s
`, tt.aggregateTotal, tt.tags))

			atelCfg, err := parseConfig(cfg)
			if tt.wantErr {
				require.EqualError(t, err, wantErr)
				return
			}

			require.NoError(t, err)
			mCfg := &atelCfg.Profiles[0].Metric.Metrics[0]
			for _, tag := range tt.wantPreservedTags {
				require.Contains(t, mCfg.preserveTagsMap, tag)
			}
			for _, tag := range tt.wantUnpreservedTags {
				require.NotContains(t, mCfg.preserveTagsMap, tag)
			}
		})
	}
}

func TestAggregateTotalWithoutPreserveTags(t *testing.T) {
	for _, preserveTags := range []struct {
		name string
		yaml string
	}{
		{name: "omitted"},
		{name: "empty", yaml: "            preserve_tags: []\n"},
	} {
		t.Run(preserveTags.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, `
agent_telemetry:
  enabled: true
  profiles:
    - name: foo
      metric:
        metrics:
          - name: bar.zoo
            aggregate_total: true
`+preserveTags.yaml)
			atelCfg, err := parseConfig(cfg)
			require.NoError(t, err)
			mCfg := &atelCfg.Profiles[0].Metric.Metrics[0]
			metrics := []*dto.Metric{
				newCounterMetric(10),
				newCounterMetric(20, newLabelPair("emitter", "")),
				newCounterMetric(30, newLabelPair("emitter", "system-probe")),
			}

			results := (&atel{localEmitter: "agent"}).aggregateMetricTags(mCfg, dto.MetricType_COUNTER, metrics)

			require.Len(t, results, 4)
			metricsByTag := makeStableMetricMap(results)
			require.Contains(t, metricsByTag, "emitter:agent:")
			require.Equal(t, 30.0, metricsByTag["emitter:agent:"].Counter.GetValue())
			require.Contains(t, metricsByTag, "emitter:system-probe:")
			require.Equal(t, 30.0, metricsByTag["emitter:system-probe:"].Counter.GetValue())
			agentTotal := metricsByTag["emitter:agent:total:2:"]
			require.NotNil(t, agentTotal)
			require.Equal(t, []string{"emitter=agent", "total=2"}, metricLabelStrings(agentTotal))
			require.Equal(t, 30.0, agentTotal.Counter.GetValue())
			systemProbeTotal := metricsByTag["emitter:system-probe:total:1:"]
			require.NotNil(t, systemProbeTotal)
			require.Equal(t, []string{"emitter=system-probe", "total=1"}, metricLabelStrings(systemProbeTotal))
			require.Equal(t, 30.0, systemProbeTotal.Counter.GetValue())
		})
	}
}

func TestSerializedAgentMetricPayloadsAlwaysIncludeEmitter(t *testing.T) {
	metricType := dto.MetricType_COUNTER
	familyName := "test__wire_emitter"
	family := &dto.MetricFamily{Name: &familyName, Type: &metricType}
	metricConfig := &MetricConfig{
		AggregateTotal:     true,
		preserveTagsExists: true,
		preserveTagsMap:    map[string]any{"kind": struct{}{}},
	}
	sources := []*dto.Metric{
		newCounterMetric(10, newLabelPair("kind", "one")),
		newCounterMetric(20, newLabelPair("emitter", ""), newLabelPair("kind", "two")),
		newCounterMetric(30, newLabelPair("emitter", "system-probe"), newLabelPair("kind", "three")),
	}

	aggregated := (&atel{localEmitter: "agent"}).aggregateMetricTags(metricConfig, metricType, sources)
	wireSender := &senderImpl{
		payloadTemplate:             Payload{APIVersion: "v2"},
		agentMetricsPayloadTemplate: AgentMetricsPayload{Message: "Agent metrics"},
	}
	session := wireSender.startSession(context.Background())
	wireSender.sendAgentMetricPayloads(session, []*agentmetric{{
		name:    "test.wire_emitter",
		metrics: aggregated,
		family:  family,
	}})

	payloadJSON, err := json.Marshal(session.flush())
	require.NoError(t, err)
	var payload Payload
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	metricPayloads, ok := payload.Payload.([]Payload)
	require.True(t, ok)
	require.Len(t, metricPayloads, 5)

	ordinaryValues := map[string]float64{
		"agent/one":          10,
		"agent/two":          20,
		"system-probe/three": 30,
	}
	totalValues := map[string]struct {
		count string
		value float64
	}{
		"agent":        {count: "2", value: 30},
		"system-probe": {count: "1", value: 30},
	}
	ordinaryCount := 0
	totalCount := 0
	for _, metricPayload := range metricPayloads {
		metrics := metricPayload.Payload.(AgentMetricsPayload).Metrics
		require.Contains(t, metrics, "agent_metadata")
		for metricName, rawMetric := range metrics {
			if metricName == "agent_metadata" {
				continue
			}

			require.Equal(t, "test.wire_emitter", metricName)
			metric, ok := rawMetric.(MetricPayload)
			require.True(t, ok)
			require.NotNil(t, metric.Tags)
			emitterEntries := 0
			for tagName, tagValue := range metric.Tags {
				if tagName == "emitter" {
					emitterEntries++
					require.NotEmpty(t, tagValue)
				}
			}
			require.Equal(t, 1, emitterEntries)

			emitter := metric.Tags["emitter"].(string)
			if rawTotal, isTotal := metric.Tags["total"]; isTotal {
				totalCount++
				expected, found := totalValues[emitter]
				require.True(t, found)
				require.Equal(t, expected.count, rawTotal)
				require.Equal(t, expected.value, metric.Value)
				delete(totalValues, emitter)
				continue
			}

			ordinaryCount++
			kind, found := metric.Tags["kind"]
			require.True(t, found)
			key := emitter + "/" + kind.(string)
			expected, found := ordinaryValues[key]
			require.True(t, found)
			require.Equal(t, expected, metric.Value)
			delete(ordinaryValues, key)
		}
	}

	require.Equal(t, 3, ordinaryCount)
	require.Equal(t, 2, totalCount)
	require.Empty(t, ordinaryValues)
	require.Empty(t, totalValues)
}

// TestAggregateTotalDeltaStabilityOnTimeseriesCountChange verifies that the
// aggregate_total counter delta remains correct when the number of timeseries
// changes between collection cycles. This is a regression test for a bug where
// the "total" tag value encoded the timeseries count (e.g., total="2" then
// total="3"), causing unstable delta cache keys and inflated/incorrect totals.
func TestAggregateTotalDeltaStabilityOnTimeseriesCountChange(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            exclude:
              zero_metric: true
            metrics:
              - name: bar.zoo
                aggregate_total: true
                preserve_tags:
                  - tag1
    `
	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "zoo", []string{"tag1", "tag2"}, "")

	// --- Cycle 1: only tag1=a1 has data (1 non-zero timeseries after filtering) ---
	counter.AddWithTags(100, map[string]string{"tag1": "a1", "tag2": "b1"})

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	a.start()
	r.(*runnerMock).run()

	// First cycle: cumulative == delta (no previous cache)
	require.Equal(t, 1, len(s.sentMetrics))
	require.Equal(t, 2, len(s.sentMetrics[0].metrics)) // tag1:a1 + total

	metrics1 := makeStableMetricMap(s.sentMetrics[0].metrics)
	require.Contains(t, metrics1, "emitter:agent:tag1:a1:")
	assert.Equal(t, float64(100), metrics1["emitter:agent:tag1:a1:"].Counter.GetValue())

	// The agent total should equal the partition sum (100), with a raw-series count of 1.
	require.Contains(t, metrics1, "emitter:agent:total:1:")
	assert.Equal(t, []string{"emitter=agent", "total=1"}, metricLabelStrings(metrics1["emitter:agent:total:1:"]))
	assert.Equal(t, float64(100), metrics1["emitter:agent:total:1:"].Counter.GetValue())

	// --- Cycle 2: add a NEW timeseries tag1=a2 (now 2 timeseries) ---
	// Also increment a1 so both are non-zero
	counter.AddWithTags(50, map[string]string{"tag1": "a1", "tag2": "b1"})
	counter.AddWithTags(200, map[string]string{"tag1": "a2", "tag2": "b2"})

	// Reset sender to capture only cycle 2
	s.sentMetrics = nil
	r.(*runnerMock).run()

	require.Equal(t, 1, len(s.sentMetrics))
	require.Equal(t, 3, len(s.sentMetrics[0].metrics)) // tag1:a1 + tag1:a2 + total

	metrics2 := makeStableMetricMap(s.sentMetrics[0].metrics)

	// tag1:a1 delta should be 50 (cumulative went from 100 to 150)
	require.Contains(t, metrics2, "emitter:agent:tag1:a1:")
	assert.Equal(t, float64(50), metrics2["emitter:agent:tag1:a1:"].Counter.GetValue())

	// tag1:a2 delta should be 200 (new timeseries, no previous value)
	require.Contains(t, metrics2, "emitter:agent:tag1:a2:")
	assert.Equal(t, float64(200), metrics2["emitter:agent:tag1:a2:"].Counter.GetValue())

	// The total label changes from "1" to "2" as the source-series count increases, while the
	// total delta remains the sum of partition deltas: 50 + 200 = 250.
	require.Contains(t, metrics2, "emitter:agent:total:2:")
	assert.Equal(t, []string{"emitter=agent", "total=2"}, metricLabelStrings(metrics2["emitter:agent:total:2:"]))
	totalValue := metrics2["emitter:agent:total:2:"].Counter.GetValue()
	partitionSum := metrics2["emitter:agent:tag1:a1:"].Counter.GetValue() + metrics2["emitter:agent:tag1:a2:"].Counter.GetValue()
	assert.Equal(t, partitionSum, totalValue,
		"total delta (%v) must equal sum of partition deltas (%v); mismatch indicates unstable cache key bug",
		totalValue, partitionSum)
}

func TestTwoProfilesOnTheSameScheduleGenerateSinglePayload(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.bar
                preserve_tags:
                  - tag1
        - name: bar
          metric:
            metrics:
              - name: foo.foo
                preserve_tags:
                  - tag1
    `
	// setup and initiate a tel
	tel := makeTelMock(t)
	counter1 := tel.NewCounter("bar", "bar", []string{"tag1", "tag2", "tag3"}, "")
	counter1.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})
	counter2 := tel.NewCounter("foo", "foo", []string{"tag1", "tag2", "tag3"}, "")
	counter2.AddWithTags(20, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})

	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// Get payload
	payload, err := getPayload(a)
	require.NoError(t, err)

	// -----------------------
	// for 2 profiles there are 2 metrics, but 1 payload (test is currently payload schema dependent, improve in future)
	// Single payload whcich has sub-payloads for each metric
	// 2 metrics
	metrics := payload.Payload.(AgentMetricsPayload).Metrics
	assert.Contains(t, metrics, "bar.bar")
	assert.Contains(t, metrics, "foo.foo")
}

func TestOneProfileWithOneMetricMultipleContextsGenerateTwoPayloads(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.bar
                preserve_tags:
                  - tag1
    `
	// setup and initiate atel
	tel := makeTelMock(t)
	counter1 := tel.NewCounter("bar", "bar", []string{"tag1", "tag2", "tag3"}, "")
	counter1.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})
	counter1.AddWithTags(20, map[string]string{"tag1": "a2", "tag2": "b2", "tag3": "c2"})

	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	payloadJSON, err := a.getAsJSON()
	require.NoError(t, err)
	var payload map[string]interface{}
	err = json.Unmarshal(payloadJSON, &payload)
	require.NoError(t, err)

	// -----------------------
	// for 1 profiles there are 2 metrics in 1 payload (test is currently payload schema dependent, improve in future)

	// One payloads each has the same metric (different tags)
	requestType, ok := payload["request_type"]
	require.True(t, ok)
	assert.Equal(t, "message-batch", requestType)
	metricPayloads, ok := payload["payload"].([]interface{})
	require.True(t, ok)

	// ---------
	// 2 metrics
	// 1-st
	payload1, ok := metricPayloads[0].(map[string]interface{})
	require.True(t, ok)
	requestType1, ok := payload1["request_type"]
	require.True(t, ok)
	assert.Equal(t, "agent-metrics", requestType1)
	metricsPayload1, ok := payload1["payload"].(map[string]interface{})
	require.True(t, ok)
	metrics1, ok := metricsPayload1["metrics"].(map[string]interface{})
	require.True(t, ok)
	_, ok11 := metrics1["bar.bar"]
	_, ok12 := metrics1["foo.foo"]
	assert.True(t, (ok11 && !ok12) || (!ok11 && ok12))

	// 2-nd
	payload2, ok := metricPayloads[1].(map[string]interface{})
	require.True(t, ok)
	requestType2, ok := payload2["request_type"]
	require.True(t, ok)
	assert.Equal(t, "agent-metrics", requestType2)
	metricsPayload2, ok := payload2["payload"].(map[string]interface{})
	require.True(t, ok)
	metrics2, ok := metricsPayload2["metrics"].(map[string]interface{})
	require.True(t, ok)
	_, ok21 := metrics2["bar.bar"]
	_, ok22 := metrics2["foo.foo"]
	assert.True(t, (ok21 && !ok22) || (!ok21 && ok22))

}

func TestOneProfileWithTwoMetricGenerateSinglePayloads(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foobar
          metric:
            metrics:
              - name: bar.bar
                preserve_tags:
                  - tag1
              - name: foo.foo
                preserve_tags:
                  - tag1
    `
	// setup and initiate atel
	tel := makeTelMock(t)
	counter1 := tel.NewCounter("bar", "bar", []string{"tag1", "tag2", "tag3"}, "")
	counter1.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})
	counter2 := tel.NewCounter("foo", "foo", []string{"tag1", "tag2", "tag3"}, "")
	counter2.AddWithTags(20, map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"})

	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// Get payload
	payload, err := getPayload(a)
	require.NoError(t, err)

	// -----------------------
	// for 2 profiles there are2 metrics, but 1 payload (test is currently payload schema dependent, improve in future)
	// 2 metrics
	metrics := payload.Payload.(AgentMetricsPayload).Metrics
	assert.Contains(t, metrics, "bar.bar")
	assert.Contains(t, metrics, "foo.foo")
}

func TestSenderConfigNoConfig(t *testing.T) {
	c := `
    agent_telemetry:
      enabled: true
    `
	sndr := makeSenderImpl(t, nil, c)

	url := buildURL(sndr.(*senderImpl).endpoints.Main)
	assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com./api/v2/apmtelemetry", url)
}

// TestSenderConfigSite tests that the site configuration is correctly used to build the endpoint URL
func TestSenderConfigOnlySites(t *testing.T) {
	ctemp := `
    site: %s
    agent_telemetry:
      enabled: true
    `
	// Probably overkill (since 2 should be sufficient), but let's test all the sites
	tests := []struct {
		site    string
		testURL string
	}{
		{"datadoghq.com", "https://instrumentation-telemetry-intake.datadoghq.com./api/v2/apmtelemetry"},
		{"datad0g.com", "https://instrumentation-telemetry-intake.datad0g.com./api/v2/apmtelemetry"},
		{"datadoghq.eu", "https://instrumentation-telemetry-intake.datadoghq.eu./api/v2/apmtelemetry"},
		{"us3.datadoghq.com", "https://instrumentation-telemetry-intake.us3.datadoghq.com./api/v2/apmtelemetry"},
		{"us5.datadoghq.com", "https://instrumentation-telemetry-intake.us5.datadoghq.com./api/v2/apmtelemetry"},
		{"ap1.datadoghq.com", "https://instrumentation-telemetry-intake.ap1.datadoghq.com./api/v2/apmtelemetry"},
	}

	for _, tt := range tests {
		c := fmt.Sprintf(ctemp, tt.site)
		sndr := makeSenderImpl(t, nil, c)
		url := buildURL(sndr.(*senderImpl).endpoints.Main)
		assert.Equal(t, tt.testURL, url)
	}
}

// TestSenderConfigAdditionalEndpoint tests that the additional endpoint configuration is correctly used to build the endpoint URL
func TestSenderConfigAdditionalEndpoint(t *testing.T) {
	c := `
    site: datadoghq.com
    api_key: foo
    agent_telemetry:
      enabled: true
      additional_endpoints:
        - api_key: bar
          host: instrumentation-telemetry-intake.us5.datadoghq.com.
    `
	sndr := makeSenderImpl(t, nil, c)
	assert.NotNil(t, sndr)

	assert.Len(t, sndr.(*senderImpl).endpoints.Endpoints, 2)
	url := buildURL(sndr.(*senderImpl).endpoints.Endpoints[0])
	assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com./api/v2/apmtelemetry", url)
	url = buildURL(sndr.(*senderImpl).endpoints.Endpoints[1])
	assert.Equal(t, "https://instrumentation-telemetry-intake.us5.datadoghq.com./api/v2/apmtelemetry", url)
}

// TestSenderConfigPartialDDUrl dd_url overrides alone
func TestSenderConfigPartialDDUrl(t *testing.T) {
	c := `
    site: datadoghq.com
    api_key: foo
    agent_telemetry:
      enabled: true
      dd_url: instrumentation-telemetry-intake.us5.datadoghq.com.
    `
	sndr := makeSenderImpl(t, nil, c)
	assert.NotNil(t, sndr)

	assert.Len(t, sndr.(*senderImpl).endpoints.Endpoints, 1)
	url := buildURL(sndr.(*senderImpl).endpoints.Endpoints[0])
	assert.Equal(t, "https://instrumentation-telemetry-intake.us5.datadoghq.com./api/v2/apmtelemetry", url)
}

// TestSenderConfigFullDDUrl dd_url overrides alone
func TestSenderConfigFullDDUrl(t *testing.T) {
	c := `
    site: datadoghq.com
    api_key: foo
    agent_telemetry:
      enabled: true
      dd_url: https://instrumentation-telemetry-intake.us5.datadoghq.com.
    `
	sndr := makeSenderImpl(t, nil, c)
	assert.NotNil(t, sndr)

	assert.Len(t, sndr.(*senderImpl).endpoints.Endpoints, 1)
	url := buildURL(sndr.(*senderImpl).endpoints.Endpoints[0])
	assert.Equal(t, "https://instrumentation-telemetry-intake.us5.datadoghq.com./api/v2/apmtelemetry", url)
}

// TestSenderConfigDDUrlWithAdditionalEndpoints dd_url overrides with additional endpoints
func TestSenderConfigDDUrlWithAdditionalEndpoints(t *testing.T) {
	c := `
    site: datadoghq.com
    api_key: foo
    agent_telemetry:
      enabled: true
      dd_url: instrumentation-telemetry-intake.us5.datadoghq.com.
      additional_endpoints:
        - api_key: bar
          host: instrumentation-telemetry-intake.us3.datadoghq.com.
    `
	sndr := makeSenderImpl(t, nil, c)
	assert.NotNil(t, sndr)

	assert.Len(t, sndr.(*senderImpl).endpoints.Endpoints, 2)
	url := buildURL(sndr.(*senderImpl).endpoints.Endpoints[0])
	assert.Equal(t, "https://instrumentation-telemetry-intake.us5.datadoghq.com./api/v2/apmtelemetry", url)
	url = buildURL(sndr.(*senderImpl).endpoints.Endpoints[1])
	assert.Equal(t, "https://instrumentation-telemetry-intake.us3.datadoghq.com./api/v2/apmtelemetry", url)
}

// TestSenderConfigDDUrlWithEmptyAdditionalPoint dd_url overrides with empty additional endpoints
func TestSenderConfigDDUrlWithEmptyAdditionalPoint(t *testing.T) {
	c := `
    site: datadoghq.com
    api_key: foo
    agent_telemetry:
      enabled: true
      dd_url: instrumentation-telemetry-intake.us5.datadoghq.com.
      additional_endpoints:
    `
	sndr := makeSenderImpl(t, nil, c)
	assert.NotNil(t, sndr)

	assert.Len(t, sndr.(*senderImpl).endpoints.Endpoints, 1)
	url := buildURL(sndr.(*senderImpl).endpoints.Endpoints[0])
	assert.Equal(t, "https://instrumentation-telemetry-intake.us5.datadoghq.com./api/v2/apmtelemetry", url)
}

// TestSenderConfigLogsNoSSL verifies that logs_no_ssl: true causes buildURL to
// produce an http:// URL. Previously buildURL hardcoded "https" and ignored
// Endpoint.UseSSL(), silently dropping all telemetry in no-SSL environments.
func TestSenderConfigLogsNoSSL(t *testing.T) {
	c := `
    api_key: foo
    agent_telemetry:
      enabled: true
      logs_dd_url: "localhost:19999"
      logs_no_ssl: true
    `
	sndr := makeSenderImpl(t, nil, c)
	assert.NotNil(t, sndr)

	assert.Len(t, sndr.(*senderImpl).endpoints.Endpoints, 1)
	url := buildURL(sndr.(*senderImpl).endpoints.Endpoints[0])
	assert.Equal(t, "http://localhost:19999/api/v2/apmtelemetry", url)
}

func TestGetAsJSONScrub(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar_auth
                preserve_tags:
                  - password
              - name: foo.bar_key
                preserve_tags:
                  - api_key
              - name: foo.bar_text
                preserve_tags:
                  - text
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	counter1 := tel.NewCounter("foo", "bar_auth", []string{"password"}, "")
	counter2 := tel.NewCounter("foo", "bar_key", []string{"api_key"}, "")
	counter3 := tel.NewCounter("foo", "bar_text", []string{"text"}, "")

	// Default scrubber scrubs at least ...
	// api key, bearer key, app key, url, password, snmp, certificate
	counter1.AddWithTags(10, map[string]string{"password": "1234567890"})
	counter2.AddWithTags(11, map[string]string{"api_key": "1234567890"})
	counter3.AddWithTags(11, map[string]string{"text": "test"})

	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// Get payload
	payload, err := getPayload(a)
	require.NoError(t, err)

	// Check the scrubbing
	metrics := payload.Payload.(AgentMetricsPayload).Metrics

	metric, ok := metrics["foo.bar_auth"]
	require.True(t, ok)
	assert.Equal(t, "********", metric.(MetricPayload).Tags["password"])
	metric, ok = metrics["foo.bar_key"]
	require.True(t, ok)
	assert.Equal(t, "********90", metric.(MetricPayload).Tags["api_key"])
	metric, ok = metrics["foo.bar_text"]
	require.True(t, ok)
	assert.Equal(t, "test", metric.(MetricPayload).Tags["text"])
}

func TestAdjustPrometheusCounterValueMultipleTags(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
                preserve_tags:
                  - tag1
                  - tag2
              - name: foo.cat
                preserve_tags:
                  - tag
              - name: zoo.bar
                preserve_tags:
                  - tag1
                  - tag2
              - name: zoo.cat
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// Set up counters across several family and metric names, with and without source labels.
	counter1 := tel.NewCounter("foo", "bar", []string{"tag1", "tag2"}, "")
	counter2 := tel.NewCounter("foo", "cat", []string{"tag"}, "")
	counter3 := tel.NewCounter("zoo", "bar", []string{"tag1", "tag2"}, "")
	counter4 := tel.NewCounter("zoo", "cat", nil, "")

	// First addition (expected values should be the same as the added values)
	counter1.AddWithTags(1, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter2.AddWithTags(2, map[string]string{"tag": "tagval"})
	counter3.AddWithTags(3, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter4.Add(4)
	payload1, err1 := getPayload(a)
	require.NoError(t, err1)
	metrics1 := payload1.Payload.(AgentMetricsPayload).Metrics
	expecVals1 := map[string]float64{
		"foo.bar": 1.0,
		"foo.cat": 2.0,
		"zoo.bar": 3.0,
		"zoo.cat": 4.0,
	}
	for ek, ev := range expecVals1 {
		v, ok := metrics1[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}

	// Second addition (expected values should be the same as the added values)
	counter1.AddWithTags(10, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter2.AddWithTags(20, map[string]string{"tag": "tagval"})
	counter3.AddWithTags(30, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter4.Add(40)
	payload2, err2 := getPayload(a)
	require.NoError(t, err2)
	metrics2 := payload2.Payload.(AgentMetricsPayload).Metrics
	expecVals2 := map[string]float64{
		"foo.bar": 10.0,
		"foo.cat": 20.0,
		"zoo.bar": 30.0,
		"zoo.cat": 40.0,
	}
	for ek, ev := range expecVals2 {
		v, ok := metrics2[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}

	// Third and fourth addition (expected values should be the sum of 3rd and 4th values)
	counter1.AddWithTags(100, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter2.AddWithTags(200, map[string]string{"tag": "tagval"})
	counter3.AddWithTags(300, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter4.Add(400)
	counter1.AddWithTags(1000, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter2.AddWithTags(2000, map[string]string{"tag": "tagval"})
	counter3.AddWithTags(3000, map[string]string{"tag1": "tag1val", "tag2": "tag2val"})
	counter4.Add(4000)
	payload34, err34 := getPayload(a)
	require.NoError(t, err34)
	metrics34 := payload34.Payload.(AgentMetricsPayload).Metrics
	expecVals34 := map[string]float64{
		"foo.bar": 1100.0,
		"foo.cat": 2200.0,
		"zoo.bar": 3300.0,
		"zoo.cat": 4400.0,
	}
	for ek, ev := range expecVals34 {
		v, ok := metrics34[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}

	// No addition (expected values should be zero)
	payload5, err5 := getPayload(a)
	require.NoError(t, err5)
	metrics5 := payload5.Payload.(AgentMetricsPayload).Metrics
	expecVals5 := map[string]float64{
		"foo.bar": 0.0,
		"foo.cat": 0.0,
		"zoo.bar": 0.0,
		"zoo.cat": 0.0,
	}
	for ek, ev := range expecVals5 {
		v, ok := metrics5[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}
}

func TestAdjustPrometheusCounterValueMultipleTagValues(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
                preserve_tags:
                  - tag
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// Set up one source-labeled counter with multiple label values.
	counter := tel.NewCounter("foo", "bar", []string{"tag"}, "")

	// First addition (expected values should be the same as the added values)
	counter.AddWithTags(1, map[string]string{"tag": "val1"})
	counter.AddWithTags(2, map[string]string{"tag": "val2"})

	ms, ok := getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 := getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 1.0)
	m2, ok2 := getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 2.0)

	// Second addition (expected values should be the same as the added values)
	counter.AddWithTags(10, map[string]string{"tag": "val1"})
	counter.AddWithTags(20, map[string]string{"tag": "val2"})
	ms, ok = getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 = getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 10.0)
	m2, ok2 = getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 20.0)

	// Third and fourth addition (expected values should be the sum of 3rd and 4th values)
	counter.AddWithTags(100, map[string]string{"tag": "val1"})
	counter.AddWithTags(200, map[string]string{"tag": "val2"})
	ms, ok = getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 = getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 100.0)
	m2, ok2 = getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 200.0)

	// No addition (expected values should be zero)
	ms, ok = getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 = getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 0.0)
	m2, ok2 = getPayloadMetricByTagValues(ms, map[string]interface{}{"emitter": "agent", "tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 0.0)
}

func TestAdjustPrometheusCounterValueWithoutSourceLabels(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
              - name: foo.cat
              - name: zoo.bar
              - name: zoo.cat
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// Set up counters without source labels; serialized outputs still carry emitter metadata.
	counter1 := tel.NewCounter("foo", "bar", nil, "")
	counter2 := tel.NewCounter("foo", "cat", nil, "")
	counter3 := tel.NewCounter("zoo", "bar", nil, "")
	counter4 := tel.NewCounter("zoo", "cat", nil, "")

	// First addition (expected values should be the same as the added values)
	counter1.Add(1)
	counter2.Add(2)
	counter3.Add(3)
	counter4.Add(4)
	payload1, err1 := getPayload(a)
	require.NoError(t, err1)
	metrics1 := payload1.Payload.(AgentMetricsPayload).Metrics
	expecVals1 := map[string]float64{
		"foo.bar": 1.0,
		"foo.cat": 2.0,
		"zoo.bar": 3.0,
		"zoo.cat": 4.0,
	}
	for ek, ev := range expecVals1 {
		v, ok := metrics1[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}

	// Second addition (expected values should be the same as the added values)
	counter1.Add(10)
	counter2.Add(20)
	counter3.Add(30)
	counter4.Add(40)
	payload2, err2 := getPayload(a)
	require.NoError(t, err2)
	metrics2 := payload2.Payload.(AgentMetricsPayload).Metrics
	expecVals2 := map[string]float64{
		"foo.bar": 10.0,
		"foo.cat": 20.0,
		"zoo.bar": 30.0,
		"zoo.cat": 40.0,
	}
	for ek, ev := range expecVals2 {
		v, ok := metrics2[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}

	// Third and fourth addition (expected values should be the sum of 3rd and 4th values)
	counter1.Add(100)
	counter2.Add(200)
	counter3.Add(300)
	counter4.Add(400)
	counter1.Add(1000)
	counter2.Add(2000)
	counter3.Add(3000)
	counter4.Add(4000)
	payload34, err34 := getPayload(a)
	require.NoError(t, err34)
	metrics34 := payload34.Payload.(AgentMetricsPayload).Metrics
	expecVals34 := map[string]float64{
		"foo.bar": 1100.0,
		"foo.cat": 2200.0,
		"zoo.bar": 3300.0,
		"zoo.cat": 4400.0,
	}
	for ek, ev := range expecVals34 {
		v, ok := metrics34[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}

	// No addition (expected values should be zero)
	payload5, err5 := getPayload(a)
	require.NoError(t, err5)
	metrics5 := payload5.Payload.(AgentMetricsPayload).Metrics
	expecVals5 := map[string]float64{
		"foo.bar": 0.0,
		"foo.cat": 0.0,
		"zoo.bar": 0.0,
		"zoo.cat": 0.0,
	}
	for ek, ev := range expecVals5 {
		v, ok := metrics5[ek]
		require.True(t, ok)
		assert.Equal(t, ev, v.(MetricPayload).Value)
	}
}

func TestHistogramFloatUpperBoundNormalization(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// setup and initiate atel
	hist := tel.NewHistogram("foo", "bar", nil, "", []float64{1, 2, 5, 100})
	// bucket 0 - 5
	hist.Observe(1)
	hist.Observe(1)
	hist.Observe(1)
	hist.Observe(1)
	hist.Observe(1)
	// bucket 1 - 0
	// ..
	// bucket 2 - 3
	hist.Observe(5)
	hist.Observe(5)
	hist.Observe(5)
	// bucket 4 - 6
	hist.Observe(6)
	hist.Observe(100)
	hist.Observe(100)
	hist.Observe(100)
	hist.Observe(100)
	hist.Observe(100)
	// +inf - 2
	hist.Observe(10000)
	hist.Observe(20000)

	// Test payload1
	metric1, ok := getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metric1.Buckets, 5)
	expecVals1 := map[string]uint64{
		"1":    5,
		"2":    0,
		"5":    3,
		"100":  6,
		"+Inf": 2,
	}
	for k, b := range metric1.Buckets {
		assert.Equal(t, expecVals1[k], b)
	}

	// Test payload2 (no new observations, everything is reset)
	metric2, ok := getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metric2.Buckets, 5)
	expecVals2 := map[string]uint64{
		"1":    0,
		"2":    0,
		"5":    0,
		"100":  0,
		"+Inf": 0,
	}
	for k, b := range metric2.Buckets {
		assert.Equal(t, expecVals2[k], b)
	}

	// Repeat the same observation with the same results)
	// bucket 0 - 5
	hist.Observe(1)
	hist.Observe(1)
	hist.Observe(1)
	hist.Observe(1)
	hist.Observe(1)
	// bucket 1 - 0
	// ..
	// bucket 2 - 3
	hist.Observe(5)
	hist.Observe(5)
	hist.Observe(5)
	// bucket 4 - 6
	hist.Observe(6)
	hist.Observe(100)
	hist.Observe(100)
	hist.Observe(100)
	hist.Observe(100)
	hist.Observe(100)
	// +inf - 3
	hist.Observe(10000)
	hist.Observe(20000)
	hist.Observe(30000)

	// Test payload3
	metric3, ok := getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metric3.Buckets, 5)
	expecVals3 := map[string]uint64{
		"1":    5,
		"2":    0,
		"5":    3,
		"100":  6,
		"+Inf": 3,
	}
	for k, b := range metric3.Buckets {
		assert.Equal(t, expecVals3[k], b)
	}

	// Test raw buckets, they should be still accumulated
	rawHist := hist.WithTags(nil)
	expecVals4 := []uint64{10, 10, 16, 28}
	for i, b := range rawHist.Get().Buckets {
		assert.Equal(t, expecVals4[i], b.Count)
	}
}

// The same as above but with tags (to make sure that indexing with tags works)
func TestHistogramFloatUpperBoundNormalizationWithTags(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
                preserve_tags:
                  - tag1
                  - tag2
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// setup and initiate atel
	hist := tel.NewHistogram("foo", "bar", []string{"tag1", "tag2"}, "", []float64{1, 2, 5, 100})
	// bucket 0 - 5
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	// bucket 1 - 0
	// ..
	// bucket 2 - 3
	hist.Observe(5, "val1", "val2")
	hist.Observe(5, "val1", "val2")
	hist.Observe(5, "val1", "val2")
	// bucket 4 - 6
	hist.Observe(6, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")

	// Test payload1
	metric1, ok := getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metric1.Buckets, 5)
	expecVals1 := map[string]uint64{
		"1":    5,
		"2":    0,
		"5":    3,
		"100":  6,
		"+inf": 0,
	}
	for k, b := range metric1.Buckets {
		assert.Equal(t, expecVals1[k], b)
	}

	// Test payload2 (no new observations, everything is reset)
	metric2, ok := getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metric2.Buckets, 5)
	expecVals2 := map[string]uint64{
		"1":    0,
		"2":    0,
		"5":    0,
		"100":  0,
		"+inf": 0,
	}
	for k, b := range metric2.Buckets {
		assert.Equal(t, expecVals2[k], b)
	}

	// Repeat the same observation with the same results)
	// bucket 0 - 5
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	hist.Observe(1, "val1", "val2")
	// bucket 1 - 0
	// ..
	// bucket 2 - 3
	hist.Observe(5, "val1", "val2")
	hist.Observe(5, "val1", "val2")
	hist.Observe(5, "val1", "val2")
	// bucket 4 - 6
	hist.Observe(6, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	hist.Observe(100, "val1", "val2")
	// Test payload3
	metric3, ok := getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metric3.Buckets, 5)
	expecVals3 := map[string]uint64{
		"1":    5,
		"2":    0,
		"5":    3,
		"100":  6,
		"+inf": 0,
	}
	for k, b := range metric3.Buckets {
		assert.Equal(t, expecVals3[k], b)
	}

	// Test raw buckets, they should be still accumulated
	tags := map[string]string{"tag1": "val1", "tag2": "val2"}
	rawHist := hist.WithTags(tags)
	expecVals4 := []uint64{10, 10, 16, 28}
	for i, b := range rawHist.Get().Buckets {
		assert.Equal(t, expecVals4[i], b.Count)
	}
}

func TestHistogramFloatUpperBoundNormalizationWithMultivalueTags(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
                preserve_tags:
                  - tag
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// setup and initiate atel
	hist := tel.NewHistogram("foo", "bar", []string{"tag"}, "", []float64{1, 2, 5, 100})

	// bucket 0 - 5
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	// bucket 1 - 0
	// ..
	// bucket 2 - 3
	hist.Observe(5, "val1")
	hist.Observe(5, "val1")
	hist.Observe(5, "val1")
	// bucket 4 - 6
	hist.Observe(6, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	// bucket +inf - 2
	hist.Observe(1000, "val1")
	hist.Observe(2000, "val1")

	// bucket 0 - 10
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	// bucket 1 - 5
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	// bucket 2 - 6
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	// bucket 4 - 12
	hist.Observe(6, "val2")
	hist.Observe(6, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	// bucket +inf - 4
	hist.Observe(1000, "val2")
	hist.Observe(1000, "val2")
	hist.Observe(2000, "val2")
	hist.Observe(2000, "val2")

	// Test payload1
	metrics1, ok := getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metrics1, 2)
	require.Len(t, metrics1[0].Buckets, 5)
	expecVals1 := map[string]struct {
		n1 uint64
		n2 uint64
	}{
		"1":    {5, 10},
		"2":    {0, 5},
		"5":    {3, 6},
		"100":  {6, 12},
		"+Inf": {2, 4},
	}
	metrics11, ok := getPayloadMetricByTagValues(metrics1, map[string]interface{}{"emitter": "agent", "tag": "val1"})
	require.True(t, ok)
	for k, b := range metrics11.Buckets {
		assert.Equal(t, expecVals1[k].n1, b)
	}
	metrics12, ok := getPayloadMetricByTagValues(metrics1, map[string]interface{}{"emitter": "agent", "tag": "val2"})
	require.True(t, ok)
	for k, b := range metrics12.Buckets {
		assert.Equal(t, expecVals1[k].n2, b)
	}

	// Test payload2 (no new observations, everything is reset)
	metrics2, ok := getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metrics2, 2)
	require.Len(t, metrics2[0].Buckets, 5)
	require.Len(t, metrics2[1].Buckets, 5)
	expecVals2 := map[string]struct {
		n1 uint64
		n2 uint64
	}{
		"1":    {0, 0},
		"2":    {0, 0},
		"5":    {0, 0},
		"100":  {0, 0},
		"+Inf": {0, 0},
	}
	metrics21, ok := getPayloadMetricByTagValues(metrics2, map[string]interface{}{"emitter": "agent", "tag": "val1"})
	require.True(t, ok)
	for k, b := range metrics21.Buckets {
		assert.Equal(t, expecVals2[k].n1, b)
	}
	metrics22, ok := getPayloadMetricByTagValues(metrics2, map[string]interface{}{"emitter": "agent", "tag": "val2"})
	require.True(t, ok)
	for k, b := range metrics22.Buckets {
		assert.Equal(t, expecVals2[k].n2, b)
	}

	// Repeat the same observation with the same results)
	// bucket 0 - 5
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	hist.Observe(1, "val1")
	// bucket 1 - 0
	// ..
	// bucket 2 - 3
	hist.Observe(5, "val1")
	hist.Observe(5, "val1")
	hist.Observe(5, "val1")
	// bucket 4 - 6
	hist.Observe(6, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	hist.Observe(100, "val1")
	// bucket +inf - 2
	hist.Observe(1000, "val1")
	hist.Observe(2000, "val1")

	// bucket 0 - 10
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	hist.Observe(1, "val2")
	// bucket 1 - 5
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	hist.Observe(2, "val2")
	// bucket 2 - 6
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	hist.Observe(5, "val2")
	// bucket 4 - 12
	hist.Observe(6, "val2")
	hist.Observe(6, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	hist.Observe(100, "val2")
	// bucket +inf - 4
	hist.Observe(1000, "val2")
	hist.Observe(1000, "val2")
	hist.Observe(2000, "val2")
	hist.Observe(2000, "val2")

	// Test payload3
	metrics3, ok := getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	require.Len(t, metrics3, 2)
	require.Len(t, metrics3[0].Buckets, 5)
	require.Len(t, metrics3[1].Buckets, 5)
	expecVals3 := map[string]struct {
		n1 uint64
		n2 uint64
	}{
		"1":    {5, 10},
		"2":    {0, 5},
		"5":    {3, 6},
		"100":  {6, 12},
		"+Inf": {2, 4},
	}
	metrics31, ok := getPayloadMetricByTagValues(metrics3, map[string]interface{}{"emitter": "agent", "tag": "val1"})
	require.True(t, ok)
	for k, b := range metrics31.Buckets {
		assert.Equal(t, expecVals3[k].n1, b)
	}
	metrics32, ok := getPayloadMetricByTagValues(metrics3, map[string]interface{}{"emitter": "agent", "tag": "val2"})
	require.True(t, ok)
	for k, b := range metrics32.Buckets {
		assert.Equal(t, expecVals3[k].n2, b)
	}

	// Test raw buckets, they should be still accumulated
	tags1 := map[string]string{"tag": "val1"}
	rawHist1 := hist.WithTags(tags1)
	expecVals41 := []uint64{10, 10, 16, 28}
	for i, b := range rawHist1.Get().Buckets {
		assert.Equal(t, expecVals41[i], b.Count)
	}
	tags2 := map[string]string{"tag": "val2"}
	rawHist2 := hist.WithTags(tags2)
	expecVals42 := []uint64{20, 30, 42, 66}
	for i, b := range rawHist2.Get().Buckets {
		assert.Equal(t, expecVals42[i], b.Count)
	}
}

func TestHistogramPercentile(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// setup and initiate atel
	hist := tel.NewHistogram("foo", "bar", nil, "", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	for i := 1; i <= 10; i++ {
		hist.Observe(1)
		hist.Observe(2)
		hist.Observe(3)
		hist.Observe(4)
		hist.Observe(5)
		hist.Observe(6)
		hist.Observe(7)
		hist.Observe(8)
		hist.Observe(9)
	}
	hist.Observe(10)
	hist.Observe(10)

	metric, ok := getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.NotNil(t, metric.P75)
	require.NotNil(t, metric.P95)
	require.NotNil(t, metric.P99)

	// 75% of 92 observations is 69.0 (upper bound of the 6th bucket - 7)
	// 95% of 92 observations is 87.0 (upper bound of the 8th bucket - 9)
	// 95% of 92 observations is 92.0 (upper bound of the 10th bucket - 10)
	assert.Equal(t, 7.0, *metric.P75)
	assert.Equal(t, 9.0, *metric.P95)
	assert.Equal(t, 10.0, *metric.P99)

	// Test percentile in +Inf upper bound (p75 in 10th bucket) and p95 and p99 in +Inf bucket)
	for i := 1; i <= 10; i++ {
		hist.Observe(10)
	}
	for i := 1; i <= 4; i++ {
		hist.Observe(11)
	}

	metric, ok = getPayloadMetric(a, "foo.bar")
	require.True(t, ok)
	require.NotNil(t, metric.P75)
	require.NotNil(t, metric.P95)
	require.NotNil(t, metric.P99)

	// For percentile point of view +Inf bucket upper boundary is 2x of last explicit upper boundary
	// maybe in the future it will be configurable
	assert.Equal(t, 10.0, *metric.P75)
	assert.Equal(t, 20.0, *metric.P95)
	assert.Equal(t, 20.0, *metric.P99)
}

func TestUsingPayloadCompressionInAgentTelemetrySender(t *testing.T) {
	// Run with compression (by default default)
	var cfg1 = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
    `

	tel := makeTelMock(t)
	hist := tel.NewHistogram("foo", "bar", nil, "", []float64{1, 2, 5, 100})
	hist.Observe(1)
	hist.Observe(5)
	hist.Observe(6)
	hist.Observe(100)

	// setup and initiate atel
	cl1 := newClientMock()
	s1 := makeSenderImpl(t, cl1, cfg1)
	r1 := newRunnerMock()
	a1 := getTestAtel(t, tel, cfg1, s1, cl1, r1)
	require.True(t, a1.enabled)

	// run the runner to trigger the telemetry report
	a1.start()
	r1.(*runnerMock).run()
	assert.True(t, len(cl1.(*clientMock).body) > 0)

	// Run without compression
	var cfg2 = `
    agent_telemetry:
      use_compression: false
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
                preserve_tags:
    `

	// setup and initiate atel
	cl2 := newClientMock()
	s2 := makeSenderImpl(t, cl2, cfg2)
	r2 := newRunnerMock()
	a2 := getTestAtel(t, tel, cfg2, s2, cl2, r2)
	require.True(t, a2.enabled)

	// run the runner to trigger the telemetry report
	a2.start()
	r2.(*runnerMock).run()
	assert.True(t, len(cl2.(*clientMock).body) > 0)
	decompressBody, err := zstd.Decompress(nil, cl1.(*clientMock).body)
	require.NoError(t, err)
	require.NotZero(t, len(decompressBody))

	// we cannot compare body (time stamp different and internal
	// bucket serialization, but success above and significant size differences
	// should be suffient
	compressBodyLen := len(cl1.(*clientMock).body)
	nonCompressBodyLen := len(cl2.(*clientMock).body)
	assert.True(t, float64(nonCompressBodyLen)/float64(compressBodyLen) > 1.5)
}

func TestCoalescesDefaultAndNoDefaultMetricFamiliesBeforeAggregation(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: points
          metric:
            metrics:
              - name: point.sent
                aggregate_tags:
                  - domain
                  - remote_agent
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	corePointSent := tel.NewGaugeWithOpts("point", "sent", []string{"domain"}, "", telemetry.Options{DefaultMetric: true})
	adpPointSent := tel.NewGaugeWithOpts("point", "sent", []string{"domain", "remote_agent"}, "", telemetry.Options{DefaultMetric: false})
	corePointSent.Set(5, "https://api.datadoghq.com")
	adpPointSent.Set(400, "https://api.datadoghq.com", "agent-data-plane")

	metrics, ok := getPayloadFilteredMetricList(a, "point.sent")
	require.True(t, ok)
	require.Len(t, metrics, 2)

	coreMetric, ok := getPayloadMetricByTagValues(metrics, map[string]interface{}{
		"domain":  "https://api.datadoghq.com",
		"emitter": "agent",
	})
	require.True(t, ok)
	assert.Equal(t, 5.0, coreMetric.Value)

	adpMetric, ok := getPayloadMetricByTagValues(metrics, map[string]interface{}{
		"domain":       "https://api.datadoghq.com",
		"emitter":      "agent",
		"remote_agent": "agent-data-plane",
	})
	require.True(t, ok)
	assert.Equal(t, 400.0, adpMetric.Value)
}

func TestDefaultAndNoDefaultPromRegistries(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar
              - name: bar.foo
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	gaugeFooBar := tel.NewGaugeWithOpts("foo", "bar", nil, "", telemetry.Options{DefaultMetric: false})
	gaugeBarFoo := tel.NewGaugeWithOpts("bar", "foo", nil, "", telemetry.Options{DefaultMetric: true})
	gaugeFooBar.Set(10)
	gaugeBarFoo.Set(20)

	// Test payload
	metrics := getPayloadMetricMap(a)
	require.Len(t, metrics, 2)
	m1, ok1 := metrics["foo.bar"]
	require.True(t, ok1)
	assert.Equal(t, 10.0, m1.Value)
	m2, ok2 := metrics["bar.foo"]
	require.True(t, ok2)
	assert.Equal(t, 20.0, m2.Value)
}

func TestAgentTelemetryParseDefaultConfiguration(t *testing.T) {
	c := defaultProfiles
	cfg := configmock.NewFromYAML(t, c)
	atCfg, err := parseConfig(cfg)

	require.NoError(t, err)

	assert.True(t, len(atCfg.events) > 0)
	assert.True(t, len(atCfg.schedule) > 0)
	assert.True(t, len(atCfg.Profiles) > len(atCfg.events))
}

func TestDefaultProfilesTreatEmitterAsImplicitMetadata(t *testing.T) {
	cfg := configmock.NewFromYAML(t, defaultProfiles)
	atCfg, err := parseConfig(cfg)
	require.NoError(t, err)

	expectedMetrics := map[string]struct {
		preserveTags   []string
		aggregateTotal bool
	}{
		"dogstatsd.udp_packets_bytes": {preserveTags: nil},
		"dogstatsd.uds_packets_bytes": {preserveTags: nil},
		"logs.bytes_sent":             {preserveTags: nil, aggregateTotal: true},
		"logs.encoded_bytes_sent":     {preserveTags: []string{"compression_kind"}, aggregateTotal: true},
		"point.sent":                  {preserveTags: []string{"domain"}},
		"point.dropped":               {preserveTags: []string{"domain"}},
		"transactions.input_count":    {preserveTags: []string{"domain", "endpoint"}},
		"transactions.input_bytes":    {preserveTags: []string{"domain", "endpoint"}},
		"transactions.http_errors":    {preserveTags: []string{"code", "endpoint"}},
	}
	foundMetrics := make(map[string]struct{}, len(expectedMetrics))
	var aggregateTotalMetrics []string
	for _, profile := range atCfg.Profiles {
		if profile.Metric == nil {
			continue
		}
		for _, metric := range profile.Metric.Metrics {
			require.NotContains(t, metric.PreserveTags, "emitter", metric.Name)
			if metric.AggregateTotal {
				aggregateTotalMetrics = append(aggregateTotalMetrics, metric.Name)
			}
			expected, ok := expectedMetrics[metric.Name]
			if !ok {
				continue
			}
			require.Equal(t, expected.preserveTags, metric.PreserveTags, metric.Name)
			require.Equal(t, expected.aggregateTotal, metric.AggregateTotal, metric.Name)
			foundMetrics[metric.Name] = struct{}{}
		}
	}

	require.Len(t, foundMetrics, len(expectedMetrics))
	require.ElementsMatch(t, []string{"logs.bytes_sent", "logs.encoded_bytes_sent"}, aggregateTotalMetrics)
}

func TestAgentTelemetryEventConfiguration(t *testing.T) {
	// Use nearly full
	c := `
    agent_telemetry:
      enabled: true
      profiles:
      - name: checks
        metric:
          metrics:
            - name: checks.execution_time
              preserve_tags:
                - check_name
            - name: pymem.inuse
        schedule:
          start_after: 123
          iterations: 0
          period: 456
      - name: logs-and-metrics
        metric:
          exclude:
            zero_metric: true
          metrics:
            - name: dogstatsd.udp_packets_bytes
            - name: dogstatsd.uds_packets_bytes
        schedule:
          start_after: 30
          iterations: 0
          period: 900
      - name: ondemand
        events:
          - name: agentbsod
            request_type: agent-bsod
            payload_key: agent_bsod
            message: 'Agent BSOD'
          - name: foobar
            request_type: agent-foobar
            payload_key: agent_foobar
            message: 'Agent foobar'
      - name: ondemand2
        events:
          - name: agentbsod
            request_type: agent-bsod
            payload_key: agent_bsod
            message: 'Agent BSOD'
          - name: barfoo
            request_type: agent-barfoo
            payload_key: agent_barfoo
            message: 'Agent barfoo'
    `
	cfg := configmock.NewFromYAML(t, c)
	atCfg, err := parseConfig(cfg)

	require.NoError(t, err)

	// single event map keeps unique event names
	assert.Len(t, atCfg.events, 3)
	assert.Len(t, atCfg.schedule, 2)
	assert.Len(t, atCfg.Profiles, 4)
}

func TestNoPreserveTagsAggregateSeparatelyByEmitter(t *testing.T) {
	metrics := []*dto.Metric{
		newGaugeMetric(10, newLabelPair("unlisted", "local-one")),
		newGaugeMetric(15, newLabelPair("emitter", ""), newLabelPair("unlisted", "local-two")),
		newGaugeMetric(20, newLabelPair("emitter", "agent-data-plane"), newLabelPair("unlisted", "remote-one")),
		newGaugeMetric(25, newLabelPair("emitter", "agent-data-plane"), newLabelPair("unlisted", "remote-two")),
		newGaugeMetric(30, newLabelPair("emitter", "system-probe"), newLabelPair("unlisted", "probe")),
	}

	results := (&atel{localEmitter: "agent"}).aggregateMetricTags(&MetricConfig{}, dto.MetricType_GAUGE, metrics)

	require.Len(t, results, 3)
	metricsByTag := makeStableMetricMap(results)
	require.Contains(t, metricsByTag, "emitter:agent:")
	require.Equal(t, 25.0, metricsByTag["emitter:agent:"].Gauge.GetValue())
	require.Contains(t, metricsByTag, "emitter:agent-data-plane:")
	require.Equal(t, 45.0, metricsByTag["emitter:agent-data-plane:"].Gauge.GetValue())
	require.Contains(t, metricsByTag, "emitter:system-probe:")
	require.Equal(t, 30.0, metricsByTag["emitter:system-probe:"].Gauge.GetValue())
}

func TestAggregationPreservedTagKeyDoesNotCollideOnDelimiters(t *testing.T) {
	mCfg := &MetricConfig{
		preserveTagsExists: true,
		preserveTagsMap: map[string]any{
			"a": struct{}{},
			"c": struct{}{},
			"d": struct{}{},
		},
	}
	metrics := []*dto.Metric{
		newGaugeMetric(10, newLabelPair("a", "b:c"), newLabelPair("d", "e")),
		newGaugeMetric(20, newLabelPair("a", "b"), newLabelPair("c", "d:e")),
	}

	results := (&atel{localEmitter: "agent"}).aggregateMetricTags(mCfg, dto.MetricType_GAUGE, metrics)

	require.Len(t, results, 2)
	labelsByValue := make(map[float64][]string, len(results))
	for _, result := range results {
		labelsByValue[result.Gauge.GetValue()] = metricLabelStrings(result)
	}
	require.Equal(t, []string{"a=b:c", "d=e", "emitter=agent"}, labelsByValue[10])
	require.Equal(t, []string{"a=b", "c=d:e", "emitter=agent"}, labelsByValue[20])
}

func TestNonEmitterPreserveTagFiltersAndDropsUnlistedLabels(t *testing.T) {
	mCfg := &MetricConfig{
		Name:               "bar.zoo",
		PreserveTags:       []string{"compression_kind"},
		preserveTagsExists: true,
		preserveTagsMap:    map[string]any{"compression_kind": struct{}{}},
	}
	profile := &Profile{metricsMap: map[string]*MetricConfig{"bar_zoo": mCfg}}
	metricType := dto.MetricType_GAUGE
	metricName := "bar__zoo"
	family := &dto.MetricFamily{
		Name: &metricName,
		Type: &metricType,
		Metric: []*dto.Metric{
			newGaugeMetric(10, newLabelPair("compression_kind", "zstd"), newLabelPair("unlisted", "one")),
			newGaugeMetric(20, newLabelPair("compression_kind", "zstd"), newLabelPair("unlisted", "two")),
			newGaugeMetric(100),
		},
	}

	result := (&atel{localEmitter: "agent"}).transformMetricFamily(profile, family)

	require.NotNil(t, result)
	require.Len(t, result.metrics, 1)
	require.Equal(t, []string{"compression_kind=zstd", "emitter=agent"}, metricLabelStrings(result.metrics[0]))
	require.Equal(t, 30.0, result.metrics[0].Gauge.GetValue())
}

func TestEmitterPreserveTagIsCompatibilityNoOp(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
agent_telemetry:
  enabled: true
  profiles:
    - name: foo
      metric:
        metrics:
          - name: bar.zoo
            preserve_tags:
              - emitter
`)
	atelCfg, err := parseConfig(cfg)
	require.NoError(t, err)
	profile := atelCfg.Profiles[0]
	metricType := dto.MetricType_GAUGE
	metricName := "bar__zoo"
	family := &dto.MetricFamily{
		Name: &metricName,
		Type: &metricType,
		Metric: []*dto.Metric{
			newGaugeMetric(10),
			newGaugeMetric(20, newLabelPair("emitter", "system-probe")),
		},
	}

	result := (&atel{localEmitter: "trace-agent"}).transformMetricFamily(profile, family)

	require.NotNil(t, result)
	require.Len(t, result.metrics, 2)
	metricsByTag := makeStableMetricMap(result.metrics)
	require.Contains(t, metricsByTag, "emitter:trace-agent:")
	require.Equal(t, 10.0, metricsByTag["emitter:trace-agent:"].Gauge.GetValue())
	require.Contains(t, metricsByTag, "emitter:system-probe:")
	require.Equal(t, 20.0, metricsByTag["emitter:system-probe:"].Gauge.GetValue())
}

func TestEmitterAndNonEmitterPreserveTagsUseOnlyNonEmitterForFiltering(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
agent_telemetry:
  enabled: true
  profiles:
    - name: foo
      metric:
        metrics:
          - name: bar.zoo
            preserve_tags:
              - emitter
              - compression_kind
`)
	atelCfg, err := parseConfig(cfg)
	require.NoError(t, err)
	profile := atelCfg.Profiles[0]
	metricType := dto.MetricType_GAUGE
	metricName := "bar__zoo"
	family := &dto.MetricFamily{
		Name: &metricName,
		Type: &metricType,
		Metric: []*dto.Metric{
			newGaugeMetric(10, newLabelPair("compression_kind", "zstd")),
			newGaugeMetric(20, newLabelPair("emitter", "agent-data-plane"), newLabelPair("compression_kind", "gzip")),
			newGaugeMetric(100),
			newGaugeMetric(200, newLabelPair("emitter", "system-probe")),
		},
	}

	result := (&atel{localEmitter: "agent"}).transformMetricFamily(profile, family)

	require.NotNil(t, result)
	require.Len(t, result.metrics, 2)
	metricsByTag := makeStableMetricMap(result.metrics)
	localKey := "compression_kind:zstd:emitter:agent:"
	remoteKey := "compression_kind:gzip:emitter:agent-data-plane:"
	require.Contains(t, metricsByTag, localKey)
	require.Equal(t, 10.0, metricsByTag[localKey].Gauge.GetValue())
	require.Contains(t, metricsByTag, remoteKey)
	require.Equal(t, 20.0, metricsByTag[remoteKey].Gauge.GetValue())
}

func TestEmitterCanonicalization(t *testing.T) {
	tests := []struct {
		name         string
		localEmitter string
		labels       []*dto.LabelPair
		wantEmitter  string
	}{
		{name: "missing uses default identity when local emitter is empty", wantEmitter: "agent"},
		{name: "empty uses configured local emitter", localEmitter: "trace-agent", labels: []*dto.LabelPair{newLabelPair("emitter", "")}, wantEmitter: "trace-agent"},
		{
			name:         "first non-empty duplicate wins",
			localEmitter: "agent",
			labels: []*dto.LabelPair{
				newLabelPair("emitter", ""),
				newLabelPair("emitter", "agent-data-plane"),
				newLabelPair("emitter", "system-probe"),
				newLabelPair("allowed", "value"),
			},
			wantEmitter: "agent-data-plane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mCfg := &MetricConfig{preserveTagsMap: map[string]any{"allowed": struct{}{}}}
			results := (&atel{localEmitter: tt.localEmitter}).aggregateMetricTags(mCfg, dto.MetricType_GAUGE, []*dto.Metric{newGaugeMetric(1, tt.labels...)})

			require.Len(t, results, 1)
			wantLabels := []string{"emitter=" + tt.wantEmitter}
			if tt.name == "first non-empty duplicate wins" {
				wantLabels = []string{"allowed=value", "emitter=" + tt.wantEmitter}
			}
			require.Equal(t, wantLabels, metricLabelStrings(results[0]))
		})
	}
}

func TestEmitterTagDefaultsToAgent(t *testing.T) {
	const cfg = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                preserve_tags:
                  - emitter
    `

	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "zoo", nil, "")
	counter.Add(42)

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, cfg, s, nil, r)
	require.True(t, a.enabled)

	a.start()
	r.(*runnerMock).run()

	require.Len(t, s.sentMetrics, 1)
	require.Len(t, s.sentMetrics[0].metrics, 1)
	metric := s.sentMetrics[0].metrics[0]
	require.Len(t, metric.GetLabel(), 1)
	assert.Equal(t, "emitter", metric.GetLabel()[0].GetName())
	assert.Equal(t, "agent", metric.GetLabel()[0].GetValue())
	assert.Equal(t, float64(42), metric.Counter.GetValue())
}

func TestAgentTelemetrySendRegisteredEvent(t *testing.T) {
	// Use nearly full
	var cfg = `
    agent_telemetry:
      enabled: true
      use_compression: false
      profiles:
      - name: xxx
        metric:
          metrics:
            - name: foo.bar
      - name: ondemand
        events:
          - name: agentbsod
            request_type: agent-bsod
            payload_key: agent_bsod
            message: 'Agent BSOD'
          - name: foobar
            request_type: agent-foobar
            payload_key: agent_foobar
            message: 'Agent foobar'
    `

	payloadObj := struct {
		Date     string `json:"date"`
		Offender string `json:"offender"`
		BugCheck string `json:"bugcheck"`
	}{
		Date:     "2024-30-02 17:31:12",
		Offender: "ddnpm+0x1a3",
		BugCheck: "0x7A",
	}
	// conert to json
	payload, err := json.Marshal(payloadObj)
	require.NoError(t, err)

	// setup and initiate atel
	cl := newClientMock()
	s := makeSenderImpl(t, cl, cfg)
	r := newRunnerMock()
	a := getTestAtel(t, nil, cfg, s, cl, r)
	require.True(t, a.enabled)

	a.start()
	err = a.SendEvent("agentbsod", payload)
	require.NoError(t, err)
	assert.True(t, len(cl.(*clientMock).body) > 0)

	//deserialize the payload of cl.(*clientMock).body
	var topPayload map[string]interface{}
	err = json.Unmarshal(cl.(*clientMock).body, &topPayload)
	require.NoError(t, err)
	fmt.Print(string(cl.(*clientMock).body))

	v, ok, err2 := jsonquery.RunSingleOutput(".payload.message", topPayload)
	require.NoError(t, err2)
	require.True(t, ok)
	assert.Equal(t, "Agent BSOD", v)

	v, ok, err2 = jsonquery.RunSingleOutput(".payload.agent_bsod.offender", topPayload)
	require.NoError(t, err2)
	require.True(t, ok)
	assert.Equal(t, "ddnpm+0x1a3", v)
}

func TestAgentTelemetrySendNonRegisteredEvent(t *testing.T) {
	// Use nearly full
	var cfg = `
    agent_telemetry:
      enabled: true
      use_compression: false
      profiles:
      - name: xxx
        metric:
          metrics:
            - name: foo.bar
      - name: ondemand
        events:
          - name: agentbsod
            request_type: agent-bsod
            payload_key: agentbsod
            message: 'Agent BSOD'
          - name: foobar
            request_type: agent-foobar
            payload_key: agentfoobar
            message: 'Agent foobar'
    `

	payloadObj := struct {
		Date     string `json:"date"`
		Offender string `json:"offender"`
		BugCheck string `json:"bugcheck"`
	}{
		Date:     "2024-30-02 17:31:12",
		Offender: "ddnpm+0x1a3",
		BugCheck: "0x7A",
	}
	// conert to json
	payload, err := json.Marshal(payloadObj)
	require.NoError(t, err)

	// setup and initiate atel
	cl := newClientMock()
	s := makeSenderImpl(t, cl, cfg)
	r := newRunnerMock()
	a := getTestAtel(t, nil, cfg, s, cl, r)
	require.True(t, a.enabled)

	a.start()
	err = a.SendEvent("agentbsod2", payload)
	require.Error(t, err)
}
