// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// ---------------------------------------------------
//
// This is experimental code and is subject to change.
//
// ---------------------------------------------------

package agenttelemetryimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
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
func (s *senderMock) sendAgentStatusPayload(_ *senderSession, _ map[string]interface{}) error {
	return nil
}
func (s *senderMock) sendAgentMetricPayloads(_ *senderSession, metrics []*agentmetric) error {
	s.sentMetrics = append(s.sentMetrics, metrics...)
	return nil
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

// utilities
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

func makeStatusMock(t *testing.T) status.Component {
	return fxutil.Test[status.Mock](t, fx.Options(statusimpl.MockModule()))
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

	atel := createAtel(cfg, log, tel, makeStatusMock(t), sndr, runner)
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

func TestDisableByDefault(t *testing.T) {
	o := map[string]any{"foo": "bar", "site": "foo.bar"}
	a := getTestAtel(t, nil, o, nil, nil, nil)
	assert.False(t, a.enabled)
}

func TestRun(t *testing.T) {
	r := newRunnerMock()
	o := getCommonOverrideConfig(true, "foo.bar")
	a := getTestAtel(t, nil, o, nil, nil, r)
	assert.True(t, a.enabled)

	a.start()

	// default configuration has 1 job with 2 profiles (more configurations needs to be tested)
	// will be improved in future by providing deterministic configuration
	assert.Equal(t, 1, len(r.(*runnerMock).jobs))
	assert.Equal(t, 2, len(r.(*runnerMock).jobs[0].profiles))
}

func TestReportMetricBasic(t *testing.T) {
	tel := makeTelMock(t)
	counter := tel.NewCounter("checks", "execution_time", []string{"check_name"}, "")
	counter.Inc("mycheck")

	o := getCommonOverrideConfig(true, "foo.bar")
	c := newClientMock()
	r := newRunnerMock()
	a := getTestAtel(t, tel, o, nil, c, r)
	assert.True(t, a.enabled)

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
	assert.True(t, a.enabled)

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
	assert.True(t, a.enabled)

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

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 1 metric sent
	assert.Equal(t, 1, len(s.sentMetrics))

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

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 2 metric should be sent
	assert.Equal(t, 1, len(s.sentMetrics))
	assert.Equal(t, 2, len(s.sentMetrics[0].metrics))

	// order is not deterministic, use label key to identify the metrics
	metrics := makeStableMetricMap(s.sentMetrics[0].metrics)

	// aggregated
	assert.Contains(t, metrics, "tag1:a1:")
	m1 := metrics["tag1:a1:"]
	assert.Equal(t, float64(30), m1.Counter.GetValue())

	assert.Contains(t, metrics, "tag1:a2:")
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

	// run the runner to trigger the telemetry report
	a.start()
	r.(*runnerMock).run()

	// 4 metric sent
	assert.Equal(t, 1, len(s.sentMetrics))
	assert.Equal(t, 4, len(s.sentMetrics[0].metrics))

	// order is not deterministic, use label key to identify the metrics
	metrics := makeStableMetricMap(s.sentMetrics[0].metrics)

	// aggregated
	assert.Contains(t, metrics, "tag1:a1:")
	m1 := metrics["tag1:a1:"]
	assert.Equal(t, float64(30), m1.Counter.GetValue())

	assert.Contains(t, metrics, "tag1:a2:")
	m2 := metrics["tag1:a2:"]
	assert.Equal(t, float64(30), m2.Counter.GetValue())

	assert.Contains(t, metrics, "tag1:a3:")
	m3 := metrics["tag1:a3:"]
	assert.Equal(t, float64(150), m3.Counter.GetValue())

	assert.Contains(t, metrics, "total:6:")
	m4 := metrics["total:6:"]
	assert.Equal(t, float64(210), m4.Counter.GetValue())
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

// TestSenderConfigDDUrl dd_url overrides alone
func TestSenderConfigDDUrl(t *testing.T) {
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
