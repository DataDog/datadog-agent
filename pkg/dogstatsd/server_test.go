// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"net"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// getAvailableUDPPort requests a random port number and makes sure it is available
func getAvailableUDPPort() (int, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return -1, fmt.Errorf("can't convert udp port: %s", err)
	}

	return portInt, nil
}

func TestNewServer(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	s, err := NewServer(nil, nil, nil)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()
	assert.NotNil(t, s)
	assert.True(t, s.Started)
}

func TestStopServer(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	s, err := NewServer(nil, nil, nil)
	require.NoError(t, err, "cannot start DSD")
	s.Stop()

	// check that the port can be bound, try for 100 ms
	address, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err, "cannot resolve address")
	for i := 0; i < 10; i++ {
		var conn net.Conn
		conn, err = net.ListenUDP("udp", address)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err, "port is not available, it should be")
}

func TestUPDReceive(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	metricOut := make(chan []*metrics.MetricSample)
	eventOut := make(chan []*metrics.Event)
	serviceOut := make(chan []*metrics.ServiceCheck)
	s, err := NewServer(metricOut, eventOut, serviceOut)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]
		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon")
		assert.EqualValues(t, sample.Value, 666.0)
		assert.Equal(t, sample.Mtype, metrics.GaugeType)
		assert.ElementsMatch(t, sample.Tags, []string{"sometag1:somevalue1", "sometag2:somevalue2"})
	case <-time.After(100 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon:666|c|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]
		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon")
		assert.EqualValues(t, sample.Value, 666.0)
		assert.Equal(t, metrics.CounterType, sample.Mtype)
		assert.Equal(t, 0.5, sample.SampleRate)
	case <-time.After(100 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]
		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon")
		assert.EqualValues(t, sample.Value, 666.0)
		assert.Equal(t, metrics.HistogramType, sample.Mtype)
		assert.Equal(t, 0.5, sample.SampleRate)
	case <-time.After(100 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon:666|ms|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]
		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon")
		assert.EqualValues(t, sample.Value, 666.0)
		assert.Equal(t, metrics.HistogramType, sample.Mtype)
		assert.Equal(t, 0.5, sample.SampleRate)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon_set:abc|s|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]
		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon_set")
		assert.Equal(t, sample.RawValue, "abc")
		assert.Equal(t, sample.Mtype, metrics.SetType)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous metric
	conn.Write([]byte("daemon1:666:777|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]

		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon2")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test Service Check
	conn.Write([]byte("_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2"))
	select {
	case res := <-serviceOut:
		assert.NotNil(t, res)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous Service Check
	conn.Write([]byte("_sc|agen.down\n_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2"))
	select {
	case res := <-serviceOut:
		assert.Equal(t, 1, len(res))
		serviceCheck := res[0]
		assert.NotNil(t, serviceCheck)
		assert.Equal(t, serviceCheck.CheckName, "agent.up")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test Event
	conn.Write([]byte("_e{10,10}:test title|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	select {
	case res := <-eventOut:
		event := res[0]
		assert.NotNil(t, event)
		assert.ElementsMatch(t, event.Tags, []string{"tag1", "tag2:test"})
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous Event
	conn.Write([]byte("_e{10,0}:test title|\n_e{11,10}:test title2|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	select {
	case res := <-eventOut:
		assert.Equal(t, 1, len(res))
		event := res[0]
		assert.NotNil(t, event)
		assert.Equal(t, event.Title, "test title2")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

func TestUDPForward(t *testing.T) {
	fport, err := getAvailableUDPPort()
	require.NoError(t, err)

	// Setup UDP server to forward to
	config.Datadog.SetDefault("statsd_forward_port", fport)
	config.Datadog.SetDefault("statsd_forward_host", "127.0.0.1")

	addr := fmt.Sprintf("127.0.0.1:%d", fport)
	pc, err := net.ListenPacket("udp", addr)
	require.NoError(t, err)

	defer pc.Close()

	// Setup dogstatsd server
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	metricOut := make(chan []*metrics.MetricSample)
	eventOut := make(chan []*metrics.Event)
	serviceOut := make(chan []*metrics.ServiceCheck)
	s, err := NewServer(metricOut, eventOut, serviceOut)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Check if message is forwarded
	message := []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	conn.Write(message)

	pc.SetReadDeadline(time.Now().Add(2 * time.Second))

	buffer := make([]byte, len(message))
	_, _, err = pc.ReadFrom(buffer)
	require.NoError(t, err)

	assert.Equal(t, message, buffer)
}

func TestHistToDist(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	defaultPort := config.Datadog.GetInt("dogstatsd_port")
	config.Datadog.SetDefault("dogstatsd_port", port)
	defer config.Datadog.SetDefault("dogstatsd_port", defaultPort)
	config.Datadog.SetDefault("histogram_copy_to_distribution", true)
	defer config.Datadog.SetDefault("histogram_copy_to_distribution", false)
	config.Datadog.SetDefault("histogram_copy_to_distribution_prefix", "dist.")
	defer config.Datadog.SetDefault("histogram_copy_to_distribution_prefix", "")

	metricOut := make(chan []*metrics.MetricSample)
	eventOut := make(chan []*metrics.Event)
	serviceOut := make(chan []*metrics.ServiceCheck)
	s, err := NewServer(metricOut, eventOut, serviceOut)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|h|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case histMetrics := <-metricOut:
		assert.Equal(t, 2, len(histMetrics))
		histMetric := histMetrics[0]
		distMetric := histMetrics[1]
		assert.NotNil(t, histMetric)
		assert.Equal(t, histMetric.Name, "daemon")
		assert.EqualValues(t, histMetric.Value, 666.0)
		assert.Equal(t, metrics.HistogramType, histMetric.Mtype)

		assert.NotNil(t, distMetric)
		assert.Equal(t, distMetric.Name, "dist.daemon")
		assert.EqualValues(t, distMetric.Value, 666.0)
		assert.Equal(t, metrics.DistributionType, distMetric.Mtype)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

func TestExtraTags(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)
	config.Datadog.SetDefault("dogstatsd_tags", []string{"sometag3:somevalue3"})
	defer config.Datadog.SetDefault("dogstatsd_tags", []string{})

	metricOut := make(chan []*metrics.MetricSample)
	eventOut := make(chan []*metrics.Event)
	serviceOut := make(chan []*metrics.ServiceCheck)
	s, err := NewServer(metricOut, eventOut, serviceOut)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]
		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon")
		assert.EqualValues(t, sample.Value, 666.0)
		assert.Equal(t, sample.Mtype, metrics.GaugeType)
		assert.ElementsMatch(t, sample.Tags, []string{"sometag1:somevalue1", "sometag2:somevalue2", "sometag3:somevalue3"})
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

func TestDebugStats(t *testing.T) {
	metricOut := make(chan []*metrics.MetricSample)
	eventOut := make(chan []*metrics.Event)
	serviceOut := make(chan []*metrics.ServiceCheck)
	s, err := NewServer(metricOut, eventOut, serviceOut)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	s.storeMetricStats("some.metric1")
	s.storeMetricStats("some.metric2")
	time.Sleep(10 * time.Millisecond)
	s.storeMetricStats("some.metric1")

	data, err := s.GetJSONDebugStats()
	require.NoError(t, err, "cannot get debug stats")
	require.NotNil(t, data)
	require.NotEmpty(t, data)

	var stats map[string]metricStat
	err = json.Unmarshal(data, &stats)
	require.NoError(t, err, "data is not valid")
	require.Len(t, stats, 2, "two metrics should have been captured")

	require.True(t, stats["some.metric1"].LastSeen.After(stats["some.metric2"].LastSeen), "some.metric1 should have appeared again after sometag2")

	s.storeMetricStats("some.metric3")
	time.Sleep(10 * time.Millisecond)
	s.storeMetricStats("some.metric1")

	data, _ = s.GetJSONDebugStats()
	err = json.Unmarshal(data, &stats)
	require.NoError(t, err, "data is not valid")
	require.Len(t, stats, 3, "three metrics should have been captured")

	metric1 := stats["some.metric1"]
	metric2 := stats["some.metric2"]
	metric3 := stats["some.metric3"]
	require.True(t, metric1.LastSeen.After(metric2.LastSeen), "some.metric1 should have appeared again after some.metric2")
	require.True(t, metric1.LastSeen.After(metric3.LastSeen), "some.metric1 should have appeared again after some.metric3")
	require.True(t, metric3.LastSeen.After(metric2.LastSeen), "some.metric3 should have appeared again after some.metric2")

	require.Equal(t, metric1.Count, uint64(3))
	require.Equal(t, metric2.Count, uint64(1))
	require.Equal(t, metric3.Count, uint64(1))
}

func TestNoMappingsConfig(t *testing.T) {
	datadogYaml := ``

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	config.Datadog.SetConfigType("yaml")
	err = config.Datadog.ReadConfig(strings.NewReader(datadogYaml))
	require.NoError(t, err)

	s, err := NewServer(nil, nil, nil)
	require.NoError(t, err, "cannot start DSD")

	assert.Nil(t, s.mapper)

	packet := listeners.Packet{
		Contents: []byte("test.metric:666|g"),
		Origin:   listeners.NoOrigin,
	}
	sample, _, _ := s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	assert.Equal(t, 1, len(sample))
}

type mappingTest struct {
	Match     string
	MatchType string
	Name      string
	Tags      map[string]string
}

func TestMappingsConfig(t *testing.T) {
	datadogYaml := `
dogstatsd_mappings:
  - match: "airflow.job.duration_sec.*.*"   # metric format: airflow.job.duration_sec.<job_type>.<job_name>
    name: "airflow.job.duration"            # remap the metric name
    tags:
      job_type: "$1"
      job_name: "$2"
  - match: "airflow.job.size.*.*"   # metric format: airflow.job.duration_sec.<job_type>.<job_name>
    name: "airflow.job.size"            # remap the metric name
    tags:
      foo: "$1"
      bar: "$2"
`

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	config.Datadog.SetConfigType("yaml")
	err = config.Datadog.ReadConfig(strings.NewReader(datadogYaml))
	require.NoError(t, err)

	s, err := NewServer(nil, nil, nil)
	require.NoError(t, err, "cannot start DSD")

	expectedMappings := []mappingTest{
		{Match: "airflow.job.duration_sec.*.*", Name: "airflow.job.duration", Tags: map[string]string{"job_type": "$1", "job_name": "$2"}, MatchType: "glob"},
		{Match: "airflow.job.size.*.*", Name: "airflow.job.size", Tags: map[string]string{"foo": "$1", "bar": "$2"}, MatchType: "glob"},
	}

	var actualMappings []mappingTest
	for _, m := range s.mapper.Mappings {
		actualMappings = append(actualMappings, mappingTest{Match: m.Match, Name: m.Name, Tags: m.Tags, MatchType: m.MatchType})
	}

	assert.Equal(t, expectedMappings, actualMappings)
}

type MetricSample struct {
	Name  string
	Value float64
	Tags  []string
	Mtype metrics.MetricType
}

func TestMappingCases(t *testing.T) {
	scenarios := []struct {
		name              string
		config            string
		packets           []string
		expectedSamples   []MetricSample
		expectedCacheSize int
	}{
		{
			name: "Simple OK case",
			config: `
dogstatsd_mappings:
  - match: "test.job.duration.*.*" 
    name: "test.job.duration"
    tags:
      job_type: "$1"
      job_name: "$2"
  - match: "test.job.size.*.*"
    name: "test.job.size"
    tags:
      foo: "$1"
      bar: "$2"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name:666|g",
				"test.job.size.my_job_type.my_job_name:666|g",
				"test.job.size.not_match:666|g",
			},
			expectedSamples: []MetricSample{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.size", Tags: []string{"foo:my_job_type", "bar:my_job_name"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.size.not_match", Tags: nil, Mtype: metrics.GaugeType, Value: 666.0},
			},
			expectedCacheSize: 1000,
		},
		{
			name: "Tag already present",
			config: `
dogstatsd_mappings:
  - match: "test.job.duration.*.*" 
    name: "test.job.duration"
    tags:
      job_type: "$1"
      job_name: "$2"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name:666|g",
				"test.job.duration.my_job_type.my_job_name:666|g|#some:tag",
				"test.job.duration.my_job_type.my_job_name:666|g|#some:tag,more:tags",
			},
			expectedSamples: []MetricSample{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.duration.my_job_type.my_job_name", Tags: []string{"some:tag"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.duration.my_job_type.my_job_name", Tags: []string{"some:tag", "more:tags"}, Mtype: metrics.GaugeType, Value: 666.0},
			},
			expectedCacheSize: 1000,
		},
		{
			name: "Cache size",
			config: `
dogstatsd_mapper_cache_size: 999
dogstatsd_mappings:
  - match: "test.job.duration.*.*"
    name: "test.job.duration"
    tags:
      job_type: "$1"
      job_name: "$2"
`,
			packets:           []string{},
			expectedSamples:   nil,
			expectedCacheSize: 999,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			config.Datadog.SetConfigType("yaml")
			err := config.Datadog.ReadConfig(strings.NewReader(scenario.config))
			assert.NoError(t, err)

			port, err := getAvailableUDPPort()
			require.NoError(t, err)
			config.Datadog.SetDefault("dogstatsd_port", port)

			s, err := NewServer(nil, nil, nil)
			require.NoError(t, err)

			assert.Equal(t, config.Datadog.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize)

			var actualSamples []MetricSample
			for _, p := range scenario.packets {
				packet := listeners.Packet{
					Contents: []byte(p),
					Origin:   listeners.NoOrigin,
				}
				rawSamples, _, _ := s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})

				for _, s := range rawSamples {
					actualSamples = append(actualSamples, MetricSample{Name: s.Name, Tags: s.Tags, Mtype: s.Mtype, Value: s.Value})
				}
			}
			for _, sample := range scenario.expectedSamples {
				sort.Strings(sample.Tags)
			}
			for _, sample := range actualSamples {
				sort.Strings(sample.Tags)
			}
			assert.Equal(t, scenario.expectedSamples, actualSamples, "Case `%s` failed. `%s` should be `%s`", scenario.name, actualSamples, scenario.expectedSamples)
			s.Stop()
		})
	}
}
