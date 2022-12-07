// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
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

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")
	assert.NotNil(t, s)
	assert.True(t, s.Started)

	s.Stop()
}

func TestStopServer(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
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

// This test is proving that no data race occurred on the `cachedTlmOriginIds` map.
// It should not fail since `cachedTlmOriginIds` and `cachedOrder` should be
// properly protected from multiple accesses by `cachedTlmLock`.
// The main purpose of this test is to detect early if a future code change is
// introducing a data race.
func TestNoRaceOriginTagMaps(t *testing.T) {
	const N = 100
	s := &Server{cachedOriginCounters: make(map[string]cachedOriginCounter)}
	sync := make(chan struct{})
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		id := fmt.Sprintf("%d", i)
		go func() {
			defer func() { done <- struct{}{} }()
			<-sync
			s.getOriginCounter(id)
		}()
	}
	close(sync)
	for i := 0; i < N; i++ {
		<-done
	}
}

func TestUDPReceive(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)
	config.Datadog.Set("dogstatsd_no_aggregation_pipeline", true) // another test may have turned it off

	opts := aggregator.DefaultAgentDemultiplexerOptions(nil)
	opts.FlushInterval = 10 * time.Millisecond
	opts.DontStartForwarders = true
	opts.UseNoopEventPlatformForwarder = true
	opts.EnableNoAggregationPipeline = true

	demux := aggregator.InitTestAgentDemultiplexerWithOpts(opts)
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample := samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, sample.Mtype, metrics.GaugeType)
	assert.ElementsMatch(t, sample.Tags, []string{"sometag1:somevalue1", "sometag2:somevalue2"})
	demux.Reset()

	conn.Write([]byte("daemon:666|c|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, metrics.CounterType, sample.Mtype)
	assert.Equal(t, 0.5, sample.SampleRate)
	demux.Reset()

	conn.Write([]byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, metrics.HistogramType, sample.Mtype)
	assert.Equal(t, 0.5, sample.SampleRate)
	demux.Reset()

	conn.Write([]byte("daemon:666|ms|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, metrics.HistogramType, sample.Mtype)
	assert.Equal(t, 0.5, sample.SampleRate)
	demux.Reset()

	conn.Write([]byte("daemon_set:abc|s|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon_set")
	assert.Equal(t, sample.RawValue, "abc")
	assert.Equal(t, sample.Mtype, metrics.SetType)
	demux.Reset()

	// multi-metric packet
	conn.Write([]byte("daemon1:666|c\ndaemon2:1000|c"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 2)
	require.Len(t, timedSamples, 0)
	sample1 := samples[0]
	assert.NotNil(t, sample1)
	assert.Equal(t, sample1.Name, "daemon1")
	assert.EqualValues(t, sample1.Value, 666.0)
	assert.Equal(t, sample1.Mtype, metrics.CounterType)
	sample2 := samples[1]
	assert.NotNil(t, sample2)
	assert.Equal(t, sample2.Name, "daemon2")
	assert.EqualValues(t, sample2.Value, 1000.0)
	assert.Equal(t, sample2.Mtype, metrics.CounterType)
	demux.Reset()

	// multi-value packet
	conn.Write([]byte("daemon1:666:123|c\ndaemon2:1000|c"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 3)
	require.Len(t, timedSamples, 0)
	sample1 = samples[0]
	assert.NotNil(t, sample1)
	assert.Equal(t, sample1.Name, "daemon1")
	assert.EqualValues(t, sample1.Value, 666.0)
	assert.Equal(t, sample1.Mtype, metrics.CounterType)
	sample2 = samples[1]
	assert.NotNil(t, sample2)
	assert.Equal(t, sample2.Name, "daemon1")
	assert.EqualValues(t, sample2.Value, 123.0)
	assert.Equal(t, sample2.Mtype, metrics.CounterType)
	sample3 := samples[2]
	assert.NotNil(t, sample3)
	assert.Equal(t, sample3.Name, "daemon2")
	assert.EqualValues(t, sample3.Value, 1000.0)
	assert.Equal(t, sample3.Mtype, metrics.CounterType)
	demux.Reset()

	// multi-value packet with skip empty
	conn.Write([]byte("daemon1::666::123::::|c\ndaemon2:1000|c"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 3)
	require.Len(t, timedSamples, 0)
	sample1 = samples[0]
	assert.NotNil(t, sample1)
	assert.Equal(t, sample1.Name, "daemon1")
	assert.EqualValues(t, sample1.Value, 666.0)
	assert.Equal(t, sample1.Mtype, metrics.CounterType)
	sample2 = samples[1]
	assert.NotNil(t, sample2)
	assert.Equal(t, sample2.Name, "daemon1")
	assert.EqualValues(t, sample2.Value, 123.0)
	assert.Equal(t, sample2.Mtype, metrics.CounterType)
	sample3 = samples[2]
	assert.NotNil(t, sample3)
	assert.Equal(t, sample3.Name, "daemon2")
	assert.EqualValues(t, sample3.Value, 1000.0)
	assert.Equal(t, sample3.Mtype, metrics.CounterType)
	demux.Reset()

	//	// slightly malformed multi-metric packet, should still be parsed in whole
	conn.Write([]byte("daemon1:666|c\n\ndaemon2:1000|c\n"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 2)
	require.Len(t, timedSamples, 0)
	sample1 = samples[0]
	assert.NotNil(t, sample1)
	assert.Equal(t, sample1.Name, "daemon1")
	assert.EqualValues(t, sample1.Value, 666.0)
	assert.Equal(t, sample1.Mtype, metrics.CounterType)
	sample2 = samples[1]
	assert.NotNil(t, sample2)
	assert.Equal(t, sample2.Name, "daemon2")
	assert.EqualValues(t, sample2.Value, 1000.0)
	assert.Equal(t, sample2.Mtype, metrics.CounterType)
	demux.Reset()

	// Test erroneous metric
	conn.Write([]byte("daemon1:666a|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon2")
	demux.Reset()

	// Test empty metric
	conn.Write([]byte("daemon1:|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2\ndaemon3: :1:|g"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon2")
	demux.Reset()

	// Late gauge
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2|T1658328888"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 0)
	require.Len(t, timedSamples, 1)
	sample = timedSamples[0]
	require.NotNil(t, sample)
	assert.Equal(t, sample.Mtype, metrics.GaugeType)
	assert.Equal(t, sample.Name, "daemon")
	assert.Equal(t, sample.Timestamp, float64(1658328888))
	demux.Reset()

	// Late count
	conn.Write([]byte("daemon:666|c|#sometag1:somevalue1,sometag2:somevalue2|T1658328888"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 0)
	require.Len(t, timedSamples, 1)
	sample = timedSamples[0]
	require.NotNil(t, sample)
	assert.Equal(t, sample.Mtype, metrics.CounterType)
	assert.Equal(t, sample.Name, "daemon")
	assert.Equal(t, sample.Timestamp, float64(1658328888))
	demux.Reset()

	// Late metric and a normal one
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2|T1658328888\ndaemon2:666|c"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 1)
	sample = timedSamples[0]
	require.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.Equal(t, sample.Mtype, metrics.GaugeType)
	assert.Equal(t, sample.Timestamp, float64(1658328888))
	sample = samples[0]
	require.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon2")
	demux.Reset()

	// Test Service Check
	// ------------------

	eventOut, serviceOut := demux.GetEventsAndServiceChecksChannels()

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
	// ----------

	conn.Write([]byte("_e{10,10}:test title|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	select {
	case res := <-eventOut:
		event := res[0]
		assert.NotNil(t, event)
		assert.ElementsMatch(t, event.Tags, []string{"tag1", "tag2:test"})
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous Events
	conn.Write(
		[]byte("_e{0,9}:|test text\n" +
			"_e{-5,2}:abc\n" +
			"_e{11,10}:test title2|test\\ntext|" +
			"t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test",
		),
	)
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

	demux := mockDemultiplexer()
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
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

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|h|#sometag1:somevalue1,sometag2:somevalue2"))
	time.Sleep(time.Millisecond * 200) // give some time to the socket write/read
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	require.Equal(t, 2, len(samples))
	require.Equal(t, 0, len(timedSamples))
	histMetric := samples[0]
	distMetric := samples[1]
	assert.NotNil(t, histMetric)
	assert.Equal(t, histMetric.Name, "daemon")
	assert.EqualValues(t, histMetric.Value, 666.0)
	assert.Equal(t, metrics.HistogramType, histMetric.Mtype)

	assert.NotNil(t, distMetric)
	assert.Equal(t, distMetric.Name, "dist.daemon")
	assert.EqualValues(t, distMetric.Value, 666.0)
	assert.Equal(t, metrics.DistributionType, distMetric.Mtype)
	demux.Reset()
}

func TestScanLines(t *testing.T) {

	messages := []string{"foo", "bar", "baz", "quz", "hax", ""}
	packet := []byte(strings.Join(messages, "\n"))
	cnt := 0
	advance, tok, eol, err := ScanLines(packet, true)
	for tok != nil && err == nil {
		cnt++
		assert.Equal(t, eol, true)
		packet = packet[advance:]
		advance, tok, eol, err = ScanLines(packet, true)
	}

	assert.False(t, eol)
	assert.Equal(t, 5, cnt)

	cnt = 0
	packet = []byte(strings.Join(messages[0:len(messages)-1], "\n"))
	advance, tok, eol, err = ScanLines(packet, true)
	for tok != nil && err == nil {
		cnt++
		packet = packet[advance:]
		advance, tok, eol, err = ScanLines(packet, true)
	}

	assert.False(t, eol)
	assert.Equal(t, 5, cnt)

}

func TestEOLParsing(t *testing.T) {

	messages := []string{"foo", "bar", "baz", "quz", "hax", ""}
	packet := []byte(strings.Join(messages, "\n"))
	cnt := 0
	msg := nextMessage(&packet, true)
	for msg != nil {
		assert.Equal(t, string(msg), messages[cnt])
		msg = nextMessage(&packet, true)
		cnt++
	}

	assert.Equal(t, 5, cnt)

	packet = []byte(strings.Join(messages[0:len(messages)-1], "\r\n"))
	cnt = 0
	msg = nextMessage(&packet, true)
	for msg != nil {
		msg = nextMessage(&packet, true)
		cnt++
	}

	assert.Equal(t, 4, cnt)

}

func TestE2EParsing(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	s, err := NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#foo:bar\ndaemon:666|g|#foo:bar"))
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	assert.Equal(t, 2, len(samples))
	assert.Equal(t, 0, len(timedSamples))
	demux.Reset()
	demux.Stop(false)
	s.Stop()

	// EOL enabled
	config.Datadog.SetDefault("dogstatsd_eol_required", []string{"udp"})
	// reset to default
	defer config.Datadog.SetDefault("dogstatsd_eol_required", []string{})

	demux = aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	s, err = NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")

	// Test metric expecting an EOL
	conn.Write([]byte("daemon:666|g|#foo:bar\ndaemon:666|g|#foo:bar"))
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Equal(t, 1, len(samples))
	assert.Equal(t, 0, len(timedSamples))
	s.Stop()
	demux.Reset()
	demux.Stop(false)
}

func TestExtraTags(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)
	config.Datadog.SetDefault("dogstatsd_tags", []string{"sometag3:somevalue3"})
	defer config.Datadog.SetDefault("dogstatsd_tags", []string{})

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	s, err := NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	require.Equal(t, 1, len(samples))
	require.Equal(t, 0, len(timedSamples))
	sample := samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, sample.Mtype, metrics.GaugeType)
	assert.ElementsMatch(t, sample.Tags, []string{"sometag1:somevalue1", "sometag2:somevalue2", "sometag3:somevalue3"})
}

func TestStaticTags(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)
	config.Datadog.SetDefault("dogstatsd_tags", []string{"sometag3:somevalue3"})
	config.Datadog.SetDefault("eks_fargate", true) // triggers DD_TAGS in static_tags
	config.Datadog.SetDefault("tags", []string{"from:dd_tags"})
	config.SetDetectedFeatures(config.FeatureMap{})
	defer config.Datadog.SetDefault("dogstatsd_tags", []string{})
	defer config.Datadog.SetDefault("eks_fargate", false)

	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	s, err := NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	require.Equal(t, 1, len(samples))
	require.Equal(t, 0, len(timedSamples))
	sample := samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, sample.Mtype, metrics.GaugeType)
	assert.ElementsMatch(t, sample.Tags, []string{
		"sometag1:somevalue1",
		"sometag2:somevalue2",
		"sometag3:somevalue3",
		"from:dd_tags",
	})
}

func TestDebugStatsSpike(t *testing.T) {
	assert := assert.New(t)
	demux := mockDemultiplexer()
	defer demux.Stop(false)

	s, err := NewServer(demux, false)
	clk := clock.NewMock()
	s.Debug = newDSDServerDebugWithClock(clk)

	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()
	require.NoError(t, err, "cannot start DSD")

	s.EnableMetricsStats()
	sample := metrics.MetricSample{Name: "some.metric1", Tags: make([]string, 0)}

	send := func(count int) {
		for i := 0; i < count; i++ {
			s.storeMetricStats(sample)
		}
	}

	send(10)

	clk.Add(1050 * time.Millisecond)
	send(10)

	clk.Add(1050 * time.Millisecond)
	send(10)

	clk.Add(1050 * time.Millisecond)
	send(10)

	clk.Add(1050 * time.Millisecond)
	send(500)

	// stop the debug loop to avoid data race
	s.DisableMetricsStats()
	time.Sleep(500 * time.Millisecond)

	assert.True(s.hasSpike())

	s.EnableMetricsStats()
	// This sleep is necessary as we need to wait for the goroutine function within 'EnableMetricsStats' to start.
	// If we remove the sleep, the debug loop ticker will not be triggered by the clk.Add() call and the 500 samples
	// added with 'send(500)' will be considered as if they had been added in the same second as the previous 500 samples.
	// This will lead to a spike because we have 1000 samples in 1 second instead of having 500 and 500 in 2 different seconds.
	time.Sleep(1050 * time.Millisecond)

	clk.Add(1050 * time.Millisecond)
	send(500)

	// stop the debug loop to avoid data race
	s.DisableMetricsStats()
	time.Sleep(500 * time.Millisecond)

	// it is no more considered a spike because we had another second with 500 metrics
	assert.False(s.hasSpike())
}

func TestDebugStats(t *testing.T) {
	demux := mockDemultiplexer()
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
	clk := clock.NewMock()
	s.Debug = newDSDServerDebugWithClock(clk)
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
	hash1 := keygen.Generate(sample1.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample1.Tags))
	hash2 := keygen.Generate(sample2.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample2.Tags))
	hash3 := keygen.Generate(sample3.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample3.Tags))
	hash4 := keygen.Generate(sample4.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample4.Tags))
	hash5 := keygen.Generate(sample5.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample5.Tags))

	// test ingestion and ingestion time
	s.storeMetricStats(sample1)
	s.storeMetricStats(sample2)
	clk.Add(10 * time.Millisecond)
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
	clk.Add(10 * time.Millisecond)
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
	require.Equal(t, metric4.Tags, "c b")
	require.Equal(t, metric5.Tags, "c b")
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

	demux := mockDemultiplexer()
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")

	assert.Nil(t, s.mapper)

	parser := newParser(newFloat64ListPool())
	samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "", false)
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

			demux := mockDemultiplexer()
			defer demux.Stop(false)
			s, err := NewServer(demux, false)
			require.NoError(t, err, "Case `%s` failed. NewServer should not return error %v", scenario.name, err)

			assert.Equal(t, config.Datadog.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize, "Case `%s` failed. cache_size `%s` should be `%s`", scenario.name, config.Datadog.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize)

			var actualSamples []MetricSample
			for _, p := range scenario.packets {
				parser := newParser(newFloat64ListPool())
				samples, err := s.parseMetricMessage(samples, parser, []byte(p), "", false)
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

func TestNewServerExtraTags(t *testing.T) {
	// restore env/config after having runned the test
	p := config.Datadog.Get("dogstatsd_port")
	defer func() {
		config.Datadog.SetDefault("dogstatsd_port", p)
	}()

	require := require.New(t)
	port, err := getAvailableUDPPort()
	require.NoError(err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	demux := mockDemultiplexer()
	s, err := NewServer(demux, false)
	require.NoError(err, "starting the DogStatsD server shouldn't fail")
	require.Len(s.extraTags, 0, "no tags should have been read")
	s.Stop()
	demux.Stop(false)

	// when the extraTags parameter isn't used, the DogStatsD server is not reading this env var
	t.Setenv("DD_TAGS", "hello:world")
	demux = mockDemultiplexer()
	s, err = NewServer(demux, false)
	require.NoError(err, "starting the DogStatsD server shouldn't fail")
	require.Len(s.extraTags, 0, "no tags should have been read")
	s.Stop()
	demux.Stop(false)

	// when the extraTags parameter isn't used, the DogStatsD server is automatically reading this env var for extra tags
	t.Setenv("DD_DOGSTATSD_TAGS", "hello:world extra:tags")
	demux = mockDemultiplexer()
	s, err = NewServer(demux, false)
	require.NoError(err, "starting the DogStatsD server shouldn't fail")
	require.Len(s.extraTags, 2, "two tags should have been read")
	require.Equal(s.extraTags[0], "extra:tags", "the tag extra:tags should be set")
	require.Equal(s.extraTags[1], "hello:world", "the tag hello:world should be set")
	s.Stop()
	demux.Stop(false)
}

func testProcessedMetricsOrigin(t *testing.T) {
	assert := assert.New(t)

	demux := mockDemultiplexer()
	defer demux.Stop(false)
	s, err := NewServer(demux, false)
	assert.NoError(err, "starting the DogStatsD server shouldn't fail")
	s.Stop()

	assert.Len(s.cachedOriginCounters, 0, "this cache must be empty")
	assert.Len(s.cachedOrder, 0, "this cache list must be empty")

	parser := newParser(newFloat64ListPool())
	samples := []metrics.MetricSample{}
	samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "container_id://test_container", false)
	assert.NoError(err)
	assert.Len(samples, 1)

	// one thing should have been stored when we parse a metric
	samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:555|g"), "container_id://test_container", true)
	assert.NoError(err)
	assert.Len(samples, 2)
	assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
	assert.Len(s.cachedOrder, 1, "one entry should have been cached")
	assert.Equal(s.cachedOrder[0].origin, "container_id://test_container")

	// when we parse another metric (different value) with same origin, cache should contain only one entry
	samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "container_id://test_container", true)
	assert.NoError(err)
	assert.Len(samples, 3)
	assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
	assert.Len(s.cachedOrder, 1, "one entry should have been cached")
	assert.Equal(s.cachedOrder[0].origin, "container_id://test_container")
	assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://test_container"})
	assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://test_container"})

	// when we parse another metric (different value) but with a different origin, we should store a new entry
	samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "container_id://another_container", true)
	assert.NoError(err)
	assert.Len(samples, 4)
	assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached")
	assert.Len(s.cachedOrder, 2, "two entries should have been cached")
	assert.Equal(s.cachedOrder[0].origin, "container_id://test_container")
	assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://test_container"})
	assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://test_container"})
	assert.Equal(s.cachedOrder[1].origin, "container_id://another_container")
	assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://another_container"})
	assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://another_container"})

	// oldest one should be removed once we reach the limit of the cache
	maxOriginCounters = 2
	samples, err = s.parseMetricMessage(samples, parser, []byte("yetanothermetric:525|g"), "third_origin", true)
	assert.NoError(err)
	assert.Len(samples, 5)
	assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached, one has been evicted already")
	assert.Len(s.cachedOrder, 2, "two entries should have been cached, one has been evicted already")
	assert.Equal(s.cachedOrder[0].origin, "container_id://another_container")
	assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://another_container"})
	assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://another_container"})
	assert.Equal(s.cachedOrder[1].origin, "third_origin")
	assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "third_origin"})
	assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "third_origin"})

	// oldest one should be removed once we reach the limit of the cache
	maxOriginCounters = 2
	samples, err = s.parseMetricMessage(samples, parser, []byte("blablabla:555|g"), "fourth_origin", true)
	assert.NoError(err)
	assert.Len(samples, 6)
	assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached, two have been evicted already")
	assert.Len(s.cachedOrder, 2, "two entries should have been cached, two have been evicted already")
	assert.Equal(s.cachedOrder[0].origin, "third_origin")
	assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "third_origin"})
	assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "third_origin"})
	assert.Equal(s.cachedOrder[1].origin, "fourth_origin")
	assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "fourth_origin"})
	assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "fourth_origin"})
}

func TestProcessedMetricsOrigin(t *testing.T) {
	v := config.Datadog.GetBool("dogstatsd_origin_optout_enabled")
	defer config.Datadog.Set("dogstatsd_origin_optout_enabled", v)
	for _, enabled := range []bool{true, false} {
		config.Datadog.Set("dogstatsd_origin_optout_enabled", enabled)
		t.Run(fmt.Sprintf("optout_enabled=%v", enabled), testProcessedMetricsOrigin)
	}
}

func testContainerIDParsing(t *testing.T) {
	assert := assert.New(t)

	s, err := NewServer(mockDemultiplexer(), false)
	assert.NoError(err, "starting the DogStatsD server shouldn't fail")
	s.Stop()

	parser := newParser(newFloat64ListPool())
	parser.dsdOriginEnabled = true

	// Metric
	metrics, err := s.parseMetricMessage(nil, parser, []byte("metric.name:123|g|c:metric-container"), "", false)
	assert.NoError(err)
	assert.Len(metrics, 1)
	assert.Equal("container_id://metric-container", metrics[0].OriginFromClient)

	// Event
	event, err := s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	assert.NoError(err)
	assert.NotNil(event)
	assert.Equal("container_id://event-container", event.OriginFromClient)

	// Service check
	serviceCheck, err := s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	assert.NoError(err)
	assert.NotNil(serviceCheck)
	assert.Equal("container_id://service-check-container", serviceCheck.OriginFromClient)
}

func TestContainerIDParsing(t *testing.T) {
	v := config.Datadog.GetBool("dogstatsd_origin_optout_enabled")
	defer config.Datadog.Set("dogstatsd_origin_optout_enabled", v)
	for _, enabled := range []bool{true, false} {
		config.Datadog.Set("dogstatsd_origin_optout_enabled", enabled)
		t.Run(fmt.Sprintf("optout_enabled=%v", enabled), testContainerIDParsing)
	}
}

func testOriginOptout(t *testing.T, enabled bool) {
	assert := assert.New(t)

	s, err := NewServer(mockDemultiplexer(), false)
	assert.NoError(err, "starting the DogStatsD server shouldn't fail")
	s.Stop()

	parser := newParser(newFloat64ListPool())
	parser.dsdOriginEnabled = true

	// Metric
	metrics, err := s.parseMetricMessage(nil, parser, []byte("metric.name:123|g|c:metric-container|#dd.internal.card:none"), "", false)
	assert.NoError(err)
	assert.Len(metrics, 1)
	if enabled {
		assert.Equal("", metrics[0].OriginFromClient)
	} else {
		assert.Equal("container_id://metric-container", metrics[0].OriginFromClient)
	}

	// Event
	event, err := s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container|#dd.internal.card:none"), "")
	assert.NoError(err)
	assert.NotNil(event)
	if enabled {
		assert.Equal("", metrics[0].OriginFromClient)
	} else {
		assert.Equal("container_id://event-container", event.OriginFromClient)
	}

	// Service check
	serviceCheck, err := s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container|#dd.internal.card:none"), "")
	assert.NoError(err)
	assert.NotNil(serviceCheck)
	if enabled {
		assert.Equal("", serviceCheck.OriginFromClient)
	} else {
		assert.Equal("container_id://service-check-container", serviceCheck.OriginFromClient)
	}
}

func TestOriginOptout(t *testing.T) {
	v := config.Datadog.GetBool("dogstatsd_origin_optout_enabled")
	defer config.Datadog.Set("dogstatsd_origin_optout_enabled", v)
	for _, enabled := range []bool{true, false} {
		config.Datadog.Set("dogstatsd_origin_optout_enabled", enabled)
		t.Run(fmt.Sprintf("optout_enabled=%v", enabled), func(t *testing.T) {
			testOriginOptout(t, enabled)
		})
	}
}
