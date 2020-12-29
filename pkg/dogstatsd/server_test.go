// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package dogstatsd

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
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

	s, err := NewServer(mockAggregator())
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()
	assert.NotNil(t, s)
	assert.True(t, s.Started)
}

func TestStopServer(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	s, err := NewServer(mockAggregator())
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

func TestUDPReceive(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	agg := mockAggregator()
	metricOut, eventOut, serviceOut := agg.GetBufferedChannels()
	s, err := NewServer(agg)
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

	// multi-metric packet
	conn.Write([]byte("daemon1:666|c\ndaemon2:1000|c"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 2, len(res))
		sample1 := res[0]
		assert.NotNil(t, sample1)
		assert.Equal(t, sample1.Name, "daemon1")
		assert.EqualValues(t, sample1.Value, 666.0)
		assert.Equal(t, sample1.Mtype, metrics.CounterType)
		sample2 := res[1]
		assert.NotNil(t, sample2)
		assert.Equal(t, sample2.Name, "daemon2")
		assert.EqualValues(t, sample2.Value, 1000.0)
		assert.Equal(t, sample2.Mtype, metrics.CounterType)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// multi-value packet
	conn.Write([]byte("daemon1:666:123|c\ndaemon2:1000|c"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 3, len(res))
		sample1 := res[0]
		assert.NotNil(t, sample1)
		assert.Equal(t, sample1.Name, "daemon1")
		assert.EqualValues(t, sample1.Value, 666.0)
		assert.Equal(t, sample1.Mtype, metrics.CounterType)
		sample2 := res[1]
		assert.NotNil(t, sample2)
		assert.Equal(t, sample2.Name, "daemon1")
		assert.EqualValues(t, sample2.Value, 123.0)
		assert.Equal(t, sample2.Mtype, metrics.CounterType)
		sample3 := res[2]
		assert.NotNil(t, sample3)
		assert.Equal(t, sample3.Name, "daemon2")
		assert.EqualValues(t, sample3.Value, 1000.0)
		assert.Equal(t, sample3.Mtype, metrics.CounterType)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// multi-value packet with skip empty
	conn.Write([]byte("daemon1::666::123::::|c\ndaemon2:1000|c"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 3, len(res))
		sample1 := res[0]
		assert.NotNil(t, sample1)
		assert.Equal(t, sample1.Name, "daemon1")
		assert.EqualValues(t, sample1.Value, 666.0)
		assert.Equal(t, sample1.Mtype, metrics.CounterType)
		sample2 := res[1]
		assert.NotNil(t, sample2)
		assert.Equal(t, sample2.Name, "daemon1")
		assert.EqualValues(t, sample2.Value, 123.0)
		assert.Equal(t, sample2.Mtype, metrics.CounterType)
		sample3 := res[2]
		assert.NotNil(t, sample3)
		assert.Equal(t, sample3.Name, "daemon2")
		assert.EqualValues(t, sample3.Value, 1000.0)
		assert.Equal(t, sample3.Mtype, metrics.CounterType)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
	// slightly malformed multi-metric packet, should still be parsed in whole
	conn.Write([]byte("daemon1:666|c\n\ndaemon2:1000|c\n"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 2, len(res))
		sample1 := res[0]
		assert.NotNil(t, sample1)
		assert.Equal(t, sample1.Name, "daemon1")
		assert.EqualValues(t, sample1.Value, 666.0)
		assert.Equal(t, sample1.Mtype, metrics.CounterType)
		sample2 := res[1]
		assert.NotNil(t, sample2)
		assert.Equal(t, sample2.Name, "daemon2")
		assert.EqualValues(t, sample2.Value, 1000.0)
		assert.Equal(t, sample2.Mtype, metrics.CounterType)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous metric
	conn.Write([]byte("daemon1:666a|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.Equal(t, 1, len(res))
		sample := res[0]

		assert.NotNil(t, sample)
		assert.Equal(t, sample.Name, "daemon2")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test empty metric
	conn.Write([]byte("daemon1:|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2\ndaemon3: :1:|g"))
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
	conn.Write([]byte("_e{0,9}:|test text\n_e{11,10}:test title2|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
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

	agg := mockAggregator()
	s, err := NewServer(agg)
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

	agg := mockAggregator()
	metricOut, _, _ := agg.GetBufferedChannels()
	s, err := NewServer(agg)
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

	agg := mockAggregator()
	metricOut, _, _ := agg.GetBufferedChannels()
	s, err := NewServer(agg)
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

func TestDebugStatsSpike(t *testing.T) {
	assert := assert.New(t)
	agg := mockAggregator()
	s, err := NewServer(agg)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	s.EnableMetricsStats()
	sample := metrics.MetricSample{Name: "some.metric1", Tags: make([]string, 0)}

	send := func(count int) {
		for i := 0; i < count; i++ {
			s.storeMetricStats(sample)
		}
	}

	send(10)
	time.Sleep(1050 * time.Millisecond)
	send(10)
	time.Sleep(1050 * time.Millisecond)
	send(10)
	time.Sleep(1050 * time.Millisecond)
	send(10)
	time.Sleep(1050 * time.Millisecond)
	send(500)

	// stop the debug loop to avoid data race
	s.DisableMetricsStats()
	time.Sleep(500 * time.Millisecond)
	assert.True(s.hasSpike())

	s.EnableMetricsStats()
	time.Sleep(1050 * time.Millisecond)
	send(500)

	// stop the debug loop to avoid data race
	s.DisableMetricsStats()
	time.Sleep(500 * time.Millisecond)
	// it is no more considered a spike because we had another second with 500 metrics
	assert.False(s.hasSpike())
}

func TestDebugStats(t *testing.T) {
	agg := mockAggregator()
	s, err := NewServer(agg)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	s.EnableMetricsStats()

	keygen := ckey.NewKeyGenerator()

	// data
	sample1 := metrics.MetricSample{Name: "some.metric1", Tags: make([]string, 0)}
	sample2 := metrics.MetricSample{Name: "some.metric2", Tags: []string{"a"}}
	sample3 := metrics.MetricSample{Name: "some.metric3", Tags: make([]string, 0)}
	sample4 := metrics.MetricSample{Name: "some.metric4", Tags: []string{"b", "c"}}
	sample5 := metrics.MetricSample{Name: "some.metric4", Tags: []string{"c", "b"}}
	hash1 := keygen.Generate(sample1.Name, "", sample1.Tags)
	hash2 := keygen.Generate(sample2.Name, "", sample2.Tags)
	hash3 := keygen.Generate(sample3.Name, "", sample3.Tags)
	hash4 := keygen.Generate(sample4.Name, "", sample4.Tags)
	hash5 := keygen.Generate(sample5.Name, "", sample5.Tags)

	// test ingestion and ingestion time
	s.storeMetricStats(sample1)
	s.storeMetricStats(sample2)
	time.Sleep(10 * time.Millisecond)
	s.storeMetricStats(sample1)

	data, err := s.GetJSONDebugStats()
	require.NoError(t, err, "cannot get debug stats")
	require.NotNil(t, data)
	require.NotEmpty(t, data)

	var stats map[ckey.ContextKey]metricStat
	err = json.Unmarshal(data, &stats)
	require.NoError(t, err, "data is not valid")
	require.Len(t, stats, 2, "two metrics should have been captured")

	require.True(t, stats[hash1].LastSeen.After(stats[hash2].LastSeen), "some.metric1 should have appeared again after some.metric2")

	s.storeMetricStats(sample3)
	time.Sleep(10 * time.Millisecond)
	s.storeMetricStats(sample1)

	s.storeMetricStats(sample4)
	s.storeMetricStats(sample5)
	data, _ = s.GetJSONDebugStats()
	err = json.Unmarshal(data, &stats)
	require.NoError(t, err, "data is not valid")
	require.Len(t, stats, 4, "4 metrics should have been captured")

	// test stats array
	metric1 := stats[hash1]
	metric2 := stats[hash2]
	metric3 := stats[hash3]
	metric4 := stats[hash4]
	metric5 := stats[hash5]

	require.True(t, metric1.LastSeen.After(metric2.LastSeen), "some.metric1 should have appeared again after some.metric2")
	require.True(t, metric1.LastSeen.After(metric3.LastSeen), "some.metric1 should have appeared again after some.metric3")
	require.True(t, metric3.LastSeen.After(metric2.LastSeen), "some.metric3 should have appeared again after some.metric2")

	require.Equal(t, metric1.Count, uint64(3))
	require.Equal(t, metric2.Count, uint64(1))
	require.Equal(t, metric3.Count, uint64(1))

	// test context correctness
	require.Equal(t, metric4.Tags, "b c")
	require.Equal(t, metric5.Tags, "b c")
	require.Equal(t, hash4, hash5)
}

func TestNoMappingsConfig(t *testing.T) {
	datadogYaml := ``
	samples := []metrics.MetricSample{}

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	config.Datadog.SetConfigType("yaml")
	err = config.Datadog.ReadConfig(strings.NewReader(datadogYaml))
	require.NoError(t, err)

	s, err := NewServer(mockAggregator())
	require.NoError(t, err, "cannot start DSD")

	assert.Nil(t, s.mapper)

	parser := newParser(newFloat64ListPool())
	samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "")
	assert.NoError(t, err)
	assert.Len(t, samples, 1)
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
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
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
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
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
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name", "some:tag"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name", "some:tag", "more:tags"}, Mtype: metrics.GaugeType, Value: 666.0},
			},
			expectedCacheSize: 1000,
		},
		{
			name: "Cache size",
			config: `
dogstatsd_mapper_cache_size: 999
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
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

	samples := []metrics.MetricSample{}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			config.Datadog.SetConfigType("yaml")
			err := config.Datadog.ReadConfig(strings.NewReader(scenario.config))
			assert.NoError(t, err, "Case `%s` failed. ReadConfig should not return error %v", scenario.name, err)

			port, err := getAvailableUDPPort()
			require.NoError(t, err, "Case `%s` failed. getAvailableUDPPort should not return error %v", scenario.name, err)
			config.Datadog.SetDefault("dogstatsd_port", port)

			s, err := NewServer(mockAggregator())
			require.NoError(t, err, "Case `%s` failed. NewServer should not return error %v", scenario.name, err)

			assert.Equal(t, config.Datadog.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize, "Case `%s` failed. cache_size `%s` should be `%s`", scenario.name, config.Datadog.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize)

			var actualSamples []MetricSample
			for _, p := range scenario.packets {
				parser := newParser(newFloat64ListPool())
				samples, err := s.parseMetricMessage(samples, parser, []byte(p), "")
				assert.NoError(t, err, "Case `%s` failed. parseMetricMessage should not return error %v", err)
				for _, sample := range samples {
					actualSamples = append(actualSamples, MetricSample{Name: sample.Name, Tags: sample.Tags, Mtype: sample.Mtype, Value: sample.Value})
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
