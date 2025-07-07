// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/zstd"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
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

// Runner mock (TODO: use use mock.Mock)
type runnerMock struct {
	mock.Mock
	jobs []job
}

func (r *runnerMock) run() {
	for _, j := range r.jobs {
		j.a.run(j.profiles)
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
		tagsKey := ""

		// sort by names and values before insertion
		origTags := m.GetLabel()
		if len(origTags) > 0 {
			for _, t := range cloneLabelsSorted(origTags) {
				tagsKey += makeLabelPairKey(t)
			}
		}

		metricMap[tagsKey] = m
	}

	return metricMap
}

func makeTelMock(t *testing.T) telemetry.Component {
	// Little hack. Telemetry component is not fully componentized, and relies on global registry so far
	// so we need to reset it before running the test. This is not ideal and will be improved in the future.
	tel := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
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

	atel := createAtel(cfg, log, tel, sndr, runner)
	if atel == nil {
		err = fmt.Errorf("failed to create atel")
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

func (p *Payload) UnmarshalAgentMetrics(itfPayload map[string]interface{}) error {
	var ok bool

	p.RequestType = "agent-metrics"
	p.APIVersion = itfPayload["request_type"].(string)

	var metricsItfPayload map[string]interface{}
	metricsItfPayload, ok = itfPayload["payload"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("payload not found")
	}
	var metricsItf map[string]interface{}
	metricsItf, ok = metricsItfPayload["metrics"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("metrics not found")
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
		return fmt.Errorf("payload not found")
	}

	// ensure all payloads which should be agent-metrics
	var payloads []Payload
	for _, payloadRaw := range payloadsRaw {
		itfChildPayload, ok := payloadRaw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid payload item type")
		}

		requestTypeRaw, ok := itfChildPayload["request_type"]
		if !ok {
			return fmt.Errorf("request_type not found")
		}
		requestType, ok := requestTypeRaw.(string)
		if !ok {
			return fmt.Errorf("request_type type is invalid")
		}

		if requestType != "agent-metrics" {
			return fmt.Errorf("request_type should be agent-metrics")
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
		return fmt.Errorf("request_type not found")
	}
	requestType, ok := requestTypeRaw.(string)
	if !ok {
		return fmt.Errorf("request_type type is invalid")
	}

	if requestType == "agent-metrics" {
		return p.UnmarshalAgentMetrics(itfPayload)
	}

	if requestType == "message-batch" {
		return p.UnmarshalMessageBatch(itfPayload)
	}

	return fmt.Errorf("request_type should be either agent-metrics or message-batch")
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

// If you have multiple metrics with different name (timeseries), meaning no multiple tags use getPayloadMetricMap
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

	// Default configuration has 3 jobs with different schedules:
	assert.Equal(t, 3, len(r.(*runnerMock).jobs))

	// Verify we have the expected number of profiles across all jobs
	totalProfiles := 0
	for _, job := range r.(*runnerMock).jobs {
		totalProfiles += len(job.profiles)
	}
	// Default config has 7 profiles total (checks, logs-and-metrics, database, api, ondemand, service-discovery, runtime-started, runtime-running)
	assert.Equal(t, 7, totalProfiles)
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

func TestNoTagSpecifiedAggregationCounter(t *testing.T) {
	c := `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                aggregate_tags: []
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

	// no tags
	assert.Nil(t, m.GetLabel())
}

func TestNoTagSpecifiedExplicitAggregationGauge(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                aggregate_tags: []
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

	// no tags
	assert.Nil(t, m.GetLabel())
}

func TestNoTagSpecifiedImplicitAggregationGauge(t *testing.T) {
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

	// no tags
	assert.Nil(t, m.GetLabel())
}

func TestNoTagSpecifiedAggregationHistogram(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: foo
          metric:
            metrics:
              - name: bar.zoo
                aggregate_tags: []
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

	// no tags
	assert.Nil(t, m.GetLabel())
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
                aggregate_tags:
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
	require.Contains(t, metrics, "tag1:a1:")
	m1 := metrics["tag1:a1:"]
	assert.Equal(t, float64(30), m1.Counter.GetValue())

	require.Contains(t, metrics, "tag1:a2:")
	m2 := metrics["tag1:a2:"]
	assert.Equal(t, float64(30), m2.Counter.GetValue())
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
                aggregate_tags:
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
	require.Contains(t, metrics, "tag1:a1:")
	m1 := metrics["tag1:a1:"]
	assert.Equal(t, float64(30), m1.Counter.GetValue())

	require.Contains(t, metrics, "tag1:a2:")
	m2 := metrics["tag1:a2:"]
	assert.Equal(t, float64(30), m2.Counter.GetValue())

	require.Contains(t, metrics, "tag1:a3:")
	m3 := metrics["tag1:a3:"]
	assert.Equal(t, float64(150), m3.Counter.GetValue())

	require.Contains(t, metrics, "total:6:")
	m4 := metrics["total:6:"]
	assert.Equal(t, float64(210), m4.Counter.GetValue())
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
                aggregate_tags:
                  - tag1
        - name: bar
          metric:
            metrics:
              - name: foo.foo
                aggregate_tags:
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
                aggregate_tags:
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
                aggregate_tags:
                  - tag1
              - name: foo.foo
                aggregate_tags:
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

func TestGetAsJSONScrub(t *testing.T) {
	var c = `
    agent_telemetry:
      enabled: true
      profiles:
        - name: xxx
          metric:
            metrics:
              - name: foo.bar_auth
                aggregate_tags:
                  - password
              - name: foo.bar_key
                aggregate_tags:
                  - api_key
              - name: foo.bar_text
                aggregate_tags:
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
	assert.Equal(t, "********", metric.(MetricPayload).Tags["api_key"])
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
                aggregate_tags:
                  - tag1
                  - tag2
              - name: foo.cat
                aggregate_tags:
                  - tag
              - name: zoo.bar
                aggregate_tags:
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

	// setup metrics using few family names, metric names and tag- and tag-less counters
	// to test various scenarios
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
                aggregate_tags:
                  - tag
    `

	// setup and initiate atel
	tel := makeTelMock(t)
	s := makeSenderImpl(t, nil, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, c, s, nil, r)
	require.True(t, a.enabled)

	// setup metrics using few family names, metric names and tag- and tag-less counters
	// to test various scenarios
	counter := tel.NewCounter("foo", "bar", []string{"tag"}, "")

	// First addition (expected values should be the same as the added values)
	counter.AddWithTags(1, map[string]string{"tag": "val1"})
	counter.AddWithTags(2, map[string]string{"tag": "val2"})

	ms, ok := getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 := getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 1.0)
	m2, ok2 := getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 2.0)

	// Second addition (expected values should be the same as the added values)
	counter.AddWithTags(10, map[string]string{"tag": "val1"})
	counter.AddWithTags(20, map[string]string{"tag": "val2"})
	ms, ok = getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 = getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 10.0)
	m2, ok2 = getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 20.0)

	// Third and fourth addition (expected values should be the sum of 3rd and 4th values)
	counter.AddWithTags(100, map[string]string{"tag": "val1"})
	counter.AddWithTags(200, map[string]string{"tag": "val2"})
	ms, ok = getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 = getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 100.0)
	m2, ok2 = getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 200.0)

	// No addition (expected values should be zero)
	ms, ok = getPayloadFilteredMetricList(a, "foo.bar")
	require.True(t, ok)
	m1, ok1 = getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val1"})
	require.True(t, ok1)
	assert.Equal(t, m1.Value, 0.0)
	m2, ok2 = getPayloadMetricByTagValues(ms, map[string]interface{}{"tag": "val2"})
	require.True(t, ok2)
	assert.Equal(t, m2.Value, 0.0)
}

func TestAdjustPrometheusCounterValueTagless(t *testing.T) {
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

	// setup metrics using few family names, metric names and tag- and tag-less counters
	// to test various scenarios
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
                aggregate_tags:
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
                aggregate_tags:
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
	metrics11, ok := getPayloadMetricByTagValues(metrics1, map[string]interface{}{"tag": "val1"})
	require.True(t, ok)
	for k, b := range metrics11.Buckets {
		assert.Equal(t, expecVals1[k].n1, b)
	}
	metrics12, ok := getPayloadMetricByTagValues(metrics1, map[string]interface{}{"tag": "val2"})
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
	metrics21, ok := getPayloadMetricByTagValues(metrics2, map[string]interface{}{"tag": "val1"})
	require.True(t, ok)
	for k, b := range metrics21.Buckets {
		assert.Equal(t, expecVals2[k].n1, b)
	}
	metrics22, ok := getPayloadMetricByTagValues(metrics2, map[string]interface{}{"tag": "val2"})
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
	metrics31, ok := getPayloadMetricByTagValues(metrics3, map[string]interface{}{"tag": "val1"})
	require.True(t, ok)
	for k, b := range metrics31.Buckets {
		assert.Equal(t, expecVals3[k].n1, b)
	}
	metrics32, ok := getPayloadMetricByTagValues(metrics3, map[string]interface{}{"tag": "val2"})
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
                aggregate_tags:
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
              aggregate_tags:
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
