// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

func runWithComponent(t testing.TB, test func(Component)) {
	fxutil.Test(t, fx.Options(
		core.MockBundle,
		serverDebug.MockModule,
		fx.Supply(core.BundleParams{}),
		fx.Supply(Params{Serverless: false}),
		replay.MockModule,
		Module,
	), test)
}

func runWithComponentAndConfig(t testing.TB, overrides map[string]interface{}, test func(Component, configComponent.Component)) {
	fxutil.Test(t, fx.Options(
		core.MockBundle,
		serverDebug.MockModule,
		fx.Supply(core.BundleParams{ConfigParams: configComponent.NewParams("", configComponent.WithOverrides(overrides))}),
		fx.Supply(Params{Serverless: false}),
		replay.MockModule,
		Module,
	), test)
}

func TestNewServer(t *testing.T) {
	cfg := make(map[string]interface{})

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg["dogstatsd_port"] = port

	runWithComponentAndConfig(t, cfg, func(s Component, _ configComponent.Component) {

		demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
		defer demux.Stop(false)
		requireStart(t, s, demux)

		s.Stop()
	})
}

func TestStopServer(t *testing.T) {
	cfg := make(map[string]interface{})

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg["dogstatsd_port"] = port

	runWithComponentAndConfig(t, cfg, func(s Component, _ configComponent.Component) {

		demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
		defer demux.Stop(false)
		requireStart(t, s, demux)
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
	})
}

// This test is proving that no data race occurred on the `cachedTlmOriginIds` map.
// It should not fail since `cachedTlmOriginIds` and `cachedOrder` should be
// properly protected from multiple accesses by `cachedTlmLock`.
// The main purpose of this test is to detect early if a future code change is
// introducing a data race.
func TestNoRaceOriginTagMaps(t *testing.T) {
	const N = 100
	s := &server{cachedOriginCounters: make(map[string]cachedOriginCounter)}
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
	cfg := make(map[string]interface{})

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg["dogstatsd_port"] = port
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off

	runWithComponentAndConfig(t, cfg, func(s Component, _ configComponent.Component) {

		opts := aggregator.DefaultAgentDemultiplexerOptions(nil)
		opts.FlushInterval = 10 * time.Millisecond
		opts.DontStartForwarders = true
		opts.UseNoopEventPlatformForwarder = true
		opts.EnableNoAggregationPipeline = true

		demux := aggregator.InitTestAgentDemultiplexerWithOpts(opts)
		defer demux.Stop(false)
		requireStart(t, s, demux)
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
		samples, timedSamples = demux.WaitForNumberOfSamples(2, 0, time.Second*2)
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
		samples, timedSamples = demux.WaitForNumberOfSamples(3, 0, time.Second*2)
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
		samples, timedSamples = demux.WaitForNumberOfSamples(3, 0, time.Second*2)
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
		samples, timedSamples = demux.WaitForNumberOfSamples(2, 0, time.Second*2)
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
		samples, timedSamples = demux.WaitForNumberOfSamples(1, 1, time.Second*2)
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
	})
}

func TestUDPForward(t *testing.T) {
	cfg := make(map[string]interface{})

	fport, err := getAvailableUDPPort()
	require.NoError(t, err)

	// Setup UDP server to forward to
	cfg["statsd_forward_port"] = fport
	cfg["statsd_forward_host"] = "127.0.0.1"

	runWithComponentAndConfig(t, cfg, func(s Component, _ configComponent.Component) {

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
		requireStart(t, s, demux)
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
	})
}

func TestHistToDist(t *testing.T) {
	cfg := make(map[string]interface{})

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg["dogstatsd_port"] = port
	cfg["histogram_copy_to_distribution"] = true
	cfg["histogram_copy_to_distribution_prefix"] = "dist."

	runWithComponentAndConfig(t, cfg, func(s Component, _ configComponent.Component) {
		config.SetDetectedFeatures(config.FeatureMap{})
		defer config.SetDetectedFeatures(nil)

		demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
		defer demux.Stop(false)
		requireStart(t, s, demux)
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
	})
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
	config.SetDetectedFeatures(config.FeatureMap{})
	defer config.SetDetectedFeatures(nil)
	cfg := make(map[string]interface{})

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg["dogstatsd_port"] = port

	runWithComponentAndConfig(t, cfg, func(s Component, c configComponent.Component) {
		demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
		requireStart(t, s, demux)

		url := fmt.Sprintf("127.0.0.1:%d", c.GetInt("dogstatsd_port"))
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
	})

	// EOL enabled
	cfg["dogstatsd_eol_required"] = []string{"udp"}

	runWithComponentAndConfig(t, cfg, func(s Component, c configComponent.Component) {
		demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
		requireStart(t, s, demux)

		url := fmt.Sprintf("127.0.0.1:%d", c.GetInt("dogstatsd_port"))
		conn, err := net.Dial("udp", url)
		require.NoError(t, err, "cannot connect to DSD socket")
		defer conn.Close()

		// Test metric expecting an EOL
		conn.Write([]byte("daemon:666|g|#foo:bar\ndaemon:666|g|#foo:bar"))
		samples, timedSamples := demux.WaitForSamples(time.Second * 2)
		require.Equal(t, 1, len(samples))
		assert.Equal(t, 0, len(timedSamples))
		s.Stop()
		demux.Reset()
		demux.Stop(false)
	})
}

func TestExtraTags(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{config.EKSFargate: struct{}{}})
	defer config.SetDetectedFeatures(nil)
	cfg := make(map[string]interface{})

	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg["dogstatsd_port"] = port
	cfg["dogstatsd_tags"] = []string{"sometag3:somevalue3"}

	runWithComponentAndConfig(t, cfg, func(s Component, _ configComponent.Component) {

		demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
		requireStart(t, s, demux)
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
	})
}

func TestStaticTags(t *testing.T) {
	cfg := make(map[string]interface{})
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg["dogstatsd_port"] = port
	cfg["dogstatsd_tags"] = []string{"sometag3:somevalue3"}
	cfg["tags"] = []string{"from:dd_tags"}

	config.SetDetectedFeatures(config.FeatureMap{config.EKSFargate: struct{}{}})
	defer config.SetDetectedFeatures(nil)

	runWithComponentAndConfig(t, cfg, func(s Component, _ configComponent.Component) {

		demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
		requireStart(t, s, demux)
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
	})
}

func TestNoMappingsConfig(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{})
	defer config.SetDetectedFeatures(nil)

	runWithComponent(t, func(c Component) {
		s := c.(*server)

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
		requireStart(t, s, demux)

		assert.Nil(t, s.mapper)

		parser := newParser(newFloat64ListPool())
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "", false)
		assert.NoError(t, err)
		assert.Len(t, samples, 1)
	})
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
			config.SetDetectedFeatures(config.FeatureMap{})
			defer config.SetDetectedFeatures(nil)

			cfg := make(map[string]interface{})

			port, err := getAvailableUDPPort()
			require.NoError(t, err, "Case `%s` failed. getAvailableUDPPort should not return error %v", scenario.name, err)
			cfg["dogstatsd_port"] = port

			runWithComponentAndConfig(t, cfg, func(serv Component, conf configComponent.Component) {
				c := conf.(config.Config)

				c.SetConfigType("yaml")
				err := c.ReadConfig(strings.NewReader(scenario.config))
				assert.NoError(t, err, "Case `%s` failed. ReadConfig should not return error %v", scenario.name, err)

				s := serv.(*server)

				demux := mockDemultiplexer()
				defer demux.Stop(false)
				requireStart(t, s, demux)
				require.NoError(t, err, "Case `%s` failed. Start should not return error %v", scenario.name, err)

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
		})
	}
}

func TestNewServerExtraTags(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{})
	defer config.SetDetectedFeatures(nil)
	cfg := make(map[string]interface{})

	require := require.New(t)
	port, err := getAvailableUDPPort()
	require.NoError(err)
	cfg["dogstatsd_port"] = port

	runWithComponentAndConfig(t, cfg, func(c Component, _ configComponent.Component) {
		s := c.(*server)
		demux := mockDemultiplexer()
		requireStart(t, s, demux)
		require.Len(s.extraTags, 0, "no tags should have been read")
		s.Stop()
		demux.Stop(false)
	})

	// when the extraTags parameter isn't used, the DogStatsD server is not reading this env var
	cfg["tags"] = "hello:world"
	runWithComponentAndConfig(t, cfg, func(c Component, _ configComponent.Component) {
		s := c.(*server)
		demux := mockDemultiplexer()
		requireStart(t, s, demux)
		require.Len(s.extraTags, 0, "no tags should have been read")
		s.Stop()
		demux.Stop(false)
	})

	// when the extraTags parameter isn't used, the DogStatsD server is automatically reading this env var for extra tags
	cfg["dogstatsd_tags"] = "hello:world extra:tags"
	runWithComponentAndConfig(t, cfg, func(c Component, _ configComponent.Component) {
		s := c.(*server)
		demux := mockDemultiplexer()
		requireStart(t, s, demux)
		require.Len(s.extraTags, 2, "two tags should have been read")
		require.Equal(s.extraTags[0], "extra:tags", "the tag extra:tags should be set")
		require.Equal(s.extraTags[1], "hello:world", "the tag hello:world should be set")
		s.Stop()
		demux.Stop(false)
	})
}

func TestProcessedMetricsOrigin(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{})
	defer config.SetDetectedFeatures(nil)
	cfg := make(map[string]interface{})

	for _, enabled := range []bool{true, false} {
		cfg["dogstatsd_origin_optout_enabled"] = enabled

		runWithComponentAndConfig(t, cfg, func(c Component, _ configComponent.Component) {
			s := c.(*server)
			assert := assert.New(t)

			demux := mockDemultiplexer()
			defer demux.Stop(false)
			requireStart(t, s, demux)

			s.Stop()

			assert.Len(s.cachedOriginCounters, 0, "this cache must be empty")
			assert.Len(s.cachedOrder, 0, "this cache list must be empty")

			parser := newParser(newFloat64ListPool())
			samples := []metrics.MetricSample{}
			samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "container_id://test_container", false)
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
		})
	}
}

func testContainerIDParsing(t *testing.T, cfg map[string]interface{}) {
	runWithComponent(t, func(c Component) {
		s := c.(*server)
		assert := assert.New(t)

		requireStart(t, s, mockDemultiplexer())
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
	})
}

func TestContainerIDParsing(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{})
	defer config.SetDetectedFeatures(nil)
	cfg := make(map[string]interface{})

	for _, enabled := range []bool{true, false} {

		cfg["dogstatsd_origin_optout_enabled"] = enabled
		t.Run(fmt.Sprintf("optout_enabled=%v", enabled), func(t *testing.T) {
			testContainerIDParsing(t, cfg)
		})
	}
}

func testOriginOptout(t *testing.T, cfg map[string]interface{}, enabled bool) {
	runWithComponentAndConfig(t, cfg, func(c Component, _ configComponent.Component) {
		s := c.(*server)
		assert := assert.New(t)

		requireStart(t, s, mockDemultiplexer())
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
	})
}

func TestOriginOptout(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{})
	defer config.SetDetectedFeatures(nil)
	cfg := make(map[string]interface{})

	for _, enabled := range []bool{true, false} {

		cfg["dogstatsd_origin_optout_enabled"] = enabled
		t.Run(fmt.Sprintf("optout_enabled=%v", enabled), func(t *testing.T) {
			testOriginOptout(t, cfg, enabled)
		})
	}
}

func requireStart(t *testing.T, s Component, demux aggregator.Demultiplexer) {
	err := s.Start(demux)
	require.NoError(t, err, "cannot start DSD")
	assert.NotNil(t, s)
	assert.True(t, s.IsRunning())
}
