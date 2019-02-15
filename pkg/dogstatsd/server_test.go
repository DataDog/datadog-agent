// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"fmt"
	"net"
	"strconv"
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
