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
	"net/http"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
func convertYamlStrToMap(t *testing.T, cfgStr string) map[string]any {
	var c map[string]any
	err := yaml.Unmarshal([]byte(cfgStr), &c)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	return c
}

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

func makeCfgMock(t *testing.T, confOverrides map[string]any) config.Component {
	return fxutil.Test[config.Component](t, config.MockModule(),
		fx.Replace(config.MockParams{Overrides: confOverrides}))
}

func makeLogMock(t *testing.T) log.Component {
	return logmock.New(t)
}

func makeSenderImpl(t *testing.T, c string) sender {
	o := convertYamlStrToMap(t, c)
	cfg := makeCfgMock(t, o)
	log := makeLogMock(t)
	client := newClientMock()
	sndr, err := newSenderImpl(cfg, log, client)
	assert.NoError(t, err)
	return sndr
}

// aggregator mock function
func getTestAtel(t *testing.T,
	tel telemetry.Component,
	ovrrd map[string]any,
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

	cfg := makeCfgMock(t, ovrrd)
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

func getCommonOverrideConfig(enabled bool, site string) map[string]any {
	if site == "" {
		return map[string]any{
			"agent_telemetry.enabled": enabled,
		}
	}
	return map[string]any{
		"agent_telemetry.enabled": enabled,
		"site":                    site,
	}
}

// This is a unit test function do not use it for actual code (at least yet)
// since it is not 100% full implementation of the unmarshalling
func (p *Payload) UnmarshalJSON(b []byte) (err error) {
	var itfPayload map[string]interface{}
	if err := json.Unmarshal(b, &itfPayload); err != nil {
		return err
	}

	requestType, ok := itfPayload["request_type"]
	if !ok {
		return fmt.Errorf("request_type not found")
	}
	if requestType.(string) == "agent-metrics" {
		p.RequestType = requestType.(string)
		p.APIVersion = itfPayload["request_type"].(string)
		p.EventTime = int64(itfPayload["event_time"].(float64))
		p.DebugFlag = itfPayload["debug"].(bool)

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

	if requestType.(string) == "message-batch" {
		return fmt.Errorf("message-batch request_type is not supported yet")
	}

	return fmt.Errorf("request_type should be either agent-metrics or message-batch")
}

// ------------------------------
// Tests

func TestEnabled(t *testing.T) {
	o := getCommonOverrideConfig(true, "foo.bar")
	a := getTestAtel(t, nil, o, nil, nil, nil)
	assert.True(t, a.enabled)
}

func TestDisable(t *testing.T) {
	o := getCommonOverrideConfig(false, "foo.bar")
	a := getTestAtel(t, nil, o, nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestDisableIfFipsEnabled(t *testing.T) {
	o := map[string]any{
		"agent_telemetry.enabled": true,
		"site":                    "foo.bar",
		"fips.enabled":            true}
	a := getTestAtel(t, nil, o, nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestEnableIfFipsDisabled(t *testing.T) {
	o := map[string]any{
		"agent_telemetry.enabled": true,
		"site":                    "foo.bar",
		"fips.enabled":            false}
	a := getTestAtel(t, nil, o, nil, nil, nil)
	assert.True(t, a.enabled)
}

func TestDisableIfGovCloud(t *testing.T) {
	o := map[string]any{
		"agent_telemetry.enabled": true,
		"site":                    "ddog-gov.com"}
	a := getTestAtel(t, nil, o, nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestEnableIfNotGovCloud(t *testing.T) {
	o := map[string]any{
		"agent_telemetry.enabled": true,
		"site":                    "datadoghq.eu"}
	a := getTestAtel(t, nil, o, nil, nil, nil)
	assert.True(t, a.enabled)
}

func TestRun(t *testing.T) {
	r := newRunnerMock()
	o := getCommonOverrideConfig(true, "foo.bar")
	a := getTestAtel(t, nil, o, nil, nil, r)
	assert.True(t, a.enabled)

	a.start()

	// Default configuration has 2 job. One with 3 profiles and another with 1 profile
	// Profiles with the same schedule are lumped into the same job
	assert.Equal(t, 2, len(r.(*runnerMock).jobs))

	// The order is not deterministic
	profile0Len := len(r.(*runnerMock).jobs[0].profiles)
	profile1Len := len(r.(*runnerMock).jobs[1].profiles)
	assert.True(t, (profile0Len == 1 && profile1Len == 3) || (profile0Len == 3 && profile1Len == 1))
}

func TestReportMetricBasic(t *testing.T) {
	tel := makeTelMock(t)
	counter := tel.NewCounter("checks", "execution_time", []string{"check_name"}, "")
	counter.Inc("mycheck")

	o := getCommonOverrideConfig(true, "foo.bar")
	c := newClientMock()
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, nil, c, r)
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
	o := convertYamlStrToMap(t, c)
	a := getTestAtel(t, tel, o, s, nil, r)
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

func TestNoTagSpecifiedAggregationGauge(t *testing.T) {
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

	o := convertYamlStrToMap(t, c)
	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
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
	gauge := tel.NewHistogram("bar", "zoo", []string{"tag1", "tag2", "tag3"}, "", buckets)
	gauge.WithTags(map[string]string{"tag1": "a1", "tag2": "b1", "tag3": "c1"}).Observe(1001)
	gauge.WithTags(map[string]string{"tag1": "a2", "tag2": "b2", "tag3": "c2"}).Observe(1002)
	gauge.WithTags(map[string]string{"tag1": "a3", "tag2": "b3", "tag3": "c3"}).Observe(1003)

	o := convertYamlStrToMap(t, c)
	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
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

	o := convertYamlStrToMap(t, c)
	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
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

	o := convertYamlStrToMap(t, c)
	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
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

	o := convertYamlStrToMap(t, c)
	s := makeSenderImpl(t, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
	require.True(t, a.enabled)

	// Get payload
	payloadJSON, err := a.GetAsJSON()
	assert.NoError(t, err)
	var payload Payload
	err = json.Unmarshal(payloadJSON, &payload)
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

	o := convertYamlStrToMap(t, c)
	s := makeSenderImpl(t, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
	require.True(t, a.enabled)

	payloadJSON, err := a.GetAsJSON()
	assert.NoError(t, err)
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

	o := convertYamlStrToMap(t, c)
	s := makeSenderImpl(t, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
	require.True(t, a.enabled)

	// Get payload
	payloadJSON, err := a.GetAsJSON()
	assert.NoError(t, err)
	var payload Payload
	err = json.Unmarshal(payloadJSON, &payload)
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
	sndr := makeSenderImpl(t, c)

	url := buildURL(sndr.(*senderImpl).endpoints.Main)
	assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry", url)
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
		{"datadoghq.com", "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry"},
		{"datad0g.com", "https://instrumentation-telemetry-intake.datad0g.com/api/v2/apmtelemetry"},
		{"datadoghq.eu", "https://instrumentation-telemetry-intake.datadoghq.eu/api/v2/apmtelemetry"},
		{"us3.datadoghq.com", "https://instrumentation-telemetry-intake.us3.datadoghq.com/api/v2/apmtelemetry"},
		{"us5.datadoghq.com", "https://instrumentation-telemetry-intake.us5.datadoghq.com/api/v2/apmtelemetry"},
		{"ap1.datadoghq.com", "https://instrumentation-telemetry-intake.ap1.datadoghq.com/api/v2/apmtelemetry"},
	}

	for _, tt := range tests {
		c := fmt.Sprintf(ctemp, tt.site)
		sndr := makeSenderImpl(t, c)
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
          host: instrumentation-telemetry-intake.us5.datadoghq.com
    `
	sndr := makeSenderImpl(t, c)
	assert.NotNil(t, sndr)

	assert.Len(t, sndr.(*senderImpl).endpoints.Endpoints, 2)
	url := buildURL(sndr.(*senderImpl).endpoints.Endpoints[0])
	assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry", url)
	url = buildURL(sndr.(*senderImpl).endpoints.Endpoints[1])
	assert.Equal(t, "https://instrumentation-telemetry-intake.us5.datadoghq.com/api/v2/apmtelemetry", url)
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
	sndr := makeSenderImpl(t, c)
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
	sndr := makeSenderImpl(t, c)
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
	sndr := makeSenderImpl(t, c)
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
	sndr := makeSenderImpl(t, c)
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

	o := convertYamlStrToMap(t, c)
	s := makeSenderImpl(t, c)
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, s, nil, r)
	require.True(t, a.enabled)

	// Get payload
	payloadJSON, err := a.GetAsJSON()
	assert.NoError(t, err)
	var payload Payload
	err = json.Unmarshal(payloadJSON, &payload)
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
