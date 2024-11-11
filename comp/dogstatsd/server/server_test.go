// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/mapper"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewServer(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	requireStart(t, deps.Server)

}

func TestUDSReceiverDisabled(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = ""                    // disabled

	deps := fulfillDepsWithConfigOverride(t, cfg)
	require.False(t, deps.Server.UdsListenerRunning())
}

// This test is proving that no data race occurred on the `cachedTlmOriginIds` map.
// It should not fail since `cachedTlmOriginIds` and `cachedOrder` should be
// properly protected from multiple accesses by `cachedTlmLock`.
// The main purpose of this test is to detect early if a future code change is
// introducing a data race.
func TestNoRaceOriginTagMaps(t *testing.T) {
	const N = 100
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fxutil.Test[depsWithoutServer](t, fx.Options(
		core.MockBundle(),
		serverdebugimpl.MockModule(),
		fx.Replace(configComponent.MockParams{
			Overrides: cfg,
		}),
		fx.Supply(Params{Serverless: false}),
		replaymock.MockModule(),
		compressionimpl.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	s := newServerCompat(deps.Config, deps.Log, deps.Replay, deps.Debug, false, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry)

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

func testReceive(t *testing.T, conn net.Conn, demux demultiplexer.FakeSamplerMock) {
	// Test metric
	_, err := conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")

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

	_, err = conn.Write([]byte("daemon:666|c|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
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

	_, err = conn.Write([]byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
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

	_, err = conn.Write([]byte("daemon:666|ms|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
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

	_, err = conn.Write([]byte("daemon_set:abc|s|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
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
	_, err = conn.Write([]byte("daemon1:666|c\ndaemon2:1000|c"))
	require.NoError(t, err, "cannot write to DSD socket")
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
	_, err = conn.Write([]byte("daemon1:666:123|c\ndaemon2:1000|c"))
	require.NoError(t, err, "cannot write to DSD socket")
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
	_, err = conn.Write([]byte("daemon1::666::123::::|c\ndaemon2:1000|c"))
	require.NoError(t, err, "cannot write to DSD socket")
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
	_, err = conn.Write([]byte("daemon1:666|c\n\ndaemon2:1000|c\n"))
	require.NoError(t, err, "cannot write to DSD socket")
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
	_, err = conn.Write([]byte("daemon1:666a|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon2")
	demux.Reset()

	// Test empty metric
	_, err = conn.Write([]byte("daemon1:|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2\ndaemon3: :1:|g"))
	require.NoError(t, err, "cannot write to DSD socket")
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample = samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon2")
	demux.Reset()

	// Late gauge
	_, err = conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2|T1658328888"))
	require.NoError(t, err, "cannot write to DSD socket")
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
	_, err = conn.Write([]byte("daemon:666|c|#sometag1:somevalue1,sometag2:somevalue2|T1658328888"))
	require.NoError(t, err, "cannot write to DSD socket")
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
	_, err = conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2|T1658328888\ndaemon2:666|c"))
	require.NoError(t, err, "cannot write to DSD socket")
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

	_, err = conn.Write([]byte("_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
	select {
	case res := <-serviceOut:
		assert.NotNil(t, res)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous Service Check
	_, err = conn.Write([]byte("_sc|agen.down\n_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
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

	_, err = conn.Write([]byte("_e{10,10}:test title|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	require.NoError(t, err, "cannot write to DSD socket")
	select {
	case res := <-eventOut:
		event := res[0]
		assert.NotNil(t, event)
		assert.ElementsMatch(t, event.Tags, []string{"tag1", "tag2:test"})
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous Events
	_, err = conn.Write(
		[]byte("_e{0,9}:|test text\n" +
			"_e{-5,2}:abc\n" +
			"_e{11,10}:test title2|test\\ntext|" +
			"t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test",
		),
	)
	require.NoError(t, err, "cannot write to DSD socket")
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

func TestScanLines(t *testing.T) {
	messages := []string{"foo", "bar", "baz", "quz", "hax", ""}
	packet := []byte(strings.Join(messages, "\n"))
	cnt := 0
	advance, tok, eol, err := scanLines(packet, true)
	for tok != nil && err == nil {
		cnt++
		assert.Equal(t, eol, true)
		packet = packet[advance:]
		advance, tok, eol, err = scanLines(packet, true)
	}

	assert.False(t, eol)
	assert.Equal(t, 5, cnt)

	cnt = 0
	packet = []byte(strings.Join(messages[0:len(messages)-1], "\n"))
	advance, tok, eol, err = scanLines(packet, true)
	for tok != nil && err == nil {
		cnt++
		packet = packet[advance:]
		advance, tok, eol, err = scanLines(packet, true)
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
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	demux := deps.Demultiplexer
	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#foo:bar\ndaemon:666|g|#foo:bar"))
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	assert.Equal(t, 2, len(samples))
	assert.Equal(t, 0, len(timedSamples))
	demux.Reset()
	demux.Stop(false)

	// EOL enabled
	cfg["dogstatsd_eol_required"] = []string{"udp"}

	deps = fulfillDepsWithConfigOverride(t, cfg)
	demux = deps.Demultiplexer
	requireStart(t, deps.Server)

	conn, err = net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric expecting an EOL
	_, err = conn.Write([]byte("daemon:666|g|#foo:bar\ndaemon:666|g|#foo:bar"))
	require.NoError(t, err, "cannot write to DSD socket")
	samples, timedSamples = demux.WaitForSamples(time.Second * 2)
	require.Equal(t, 1, len(samples))
	assert.Equal(t, 0, len(timedSamples))
	demux.Reset()
}

func TestStaticTags(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_tags"] = []string{"sometag3:somevalue3"}
	cfg["tags"] = []string{"from:dd_tags"}

	env.SetFeatures(t, env.EKSFargate)
	deps := fulfillDepsWithConfigOverride(t, cfg)

	demux := deps.Demultiplexer
	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
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

func TestNoMappingsConfig(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	cw := deps.Config.(model.Writer)
	cw.SetWithoutSource("dogstatsd_port", listeners.RandomPortName)

	samples := []metrics.MetricSample{}

	requireStart(t, s)

	assert.Nil(t, s.mapper)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "", "", false)
	assert.NoError(t, err)
	assert.Len(t, samples, 1)
}

func TestNewServerExtraTags(t *testing.T) {
	cfg := make(map[string]interface{})

	require := require.New(t)
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	requireStart(t, s)
	require.Len(s.extraTags, 0, "no tags should have been read")

	// when the extraTags parameter isn't used, the DogStatsD server is not reading this env var
	cfg["tags"] = "hello:world"
	deps = fulfillDepsWithConfigOverride(t, cfg)
	s = deps.Server.(*server)
	requireStart(t, s)
	require.Len(s.extraTags, 0, "no tags should have been read")

	// when the extraTags parameter isn't used, the DogStatsD server is automatically reading this env var for extra tags
	cfg["dogstatsd_tags"] = "hello:world extra:tags"
	deps = fulfillDepsWithConfigOverride(t, cfg)
	s = deps.Server.(*server)
	requireStart(t, s)
	require.Len(s.extraTags, 2, "two tags should have been read")
	require.Equal(s.extraTags[0], "extra:tags", "the tag extra:tags should be set")
	require.Equal(s.extraTags[1], "hello:world", "the tag hello:world should be set")
}

//nolint:revive // TODO(AML) Fix revive linter
func testContainerIDParsing(t *testing.T, cfg map[string]interface{}) {
	cfg["dogstatsd_port"] = listeners.RandomPortName
	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	assert := assert.New(t)
	requireStart(t, s)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	parser.dsdOriginEnabled = true

	// Metric
	metrics, err := s.parseMetricMessage(nil, parser, []byte("metric.name:123|g|c:metric-container"), "", "", false)
	assert.NoError(err)
	assert.Len(metrics, 1)
	assert.Equal("metric-container", metrics[0].OriginInfo.ContainerID)

	// Event
	event, err := s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	assert.NoError(err)
	assert.NotNil(event)
	assert.Equal("event-container", event.OriginInfo.ContainerID)

	// Service check
	serviceCheck, err := s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	assert.NoError(err)
	assert.NotNil(serviceCheck)
	assert.Equal("service-check-container", serviceCheck.OriginInfo.ContainerID)
}

func TestContainerIDParsing(t *testing.T) {
	cfg := make(map[string]interface{})

	for _, enabled := range []bool{true, false} {

		cfg["dogstatsd_origin_optout_enabled"] = enabled
		t.Run(fmt.Sprintf("optout_enabled=%v", enabled), func(t *testing.T) {
			testContainerIDParsing(t, cfg)
		})
	}
}

func TestOrigin(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	t.Run("TestOrigin", func(t *testing.T) {
		deps := fulfillDepsWithConfigOverride(t, cfg)
		s := deps.Server.(*server)
		assert := assert.New(t)

		requireStart(t, s)

		parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
		parser.dsdOriginEnabled = true

		// Metric
		metrics, err := s.parseMetricMessage(nil, parser, []byte("metric.name:123|g|c:metric-container|#dd.internal.card:none"), "", "", false)
		assert.NoError(err)
		assert.Len(metrics, 1)
		assert.Equal("metric-container", metrics[0].OriginInfo.ContainerID)

		// Event
		event, err := s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container|#dd.internal.card:none"), "")
		assert.NoError(err)
		assert.NotNil(event)
		assert.Equal("event-container", event.OriginInfo.ContainerID)

		// Service check
		serviceCheck, err := s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container|#dd.internal.card:none"), "")
		assert.NoError(err)
		assert.NotNil(serviceCheck)
		assert.Equal("service-check-container", serviceCheck.OriginInfo.ContainerID)
	})
}

func requireStart(t *testing.T, s Component) {
	assert.NotNil(t, s)
	assert.True(t, s.IsRunning(), "server was not running")
}

func TestDogstatsdMappingProfilesOk(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - name: "airflow"
    prefix: "airflow."
    mappings:
      - match: 'airflow\.job\.duration_sec\.(.*)'
        name: "airflow.job.duration"
        match_type: "regex"
        tags:
          job_type: "$1"
          job_name: "$2"
      - match: "airflow.job.size.*.*"
        name: "airflow.job.size"
        tags:
          foo: "$1"
          bar: "$2"
  - name: "profile2"
    prefix: "profile2."
    mappings:
      - match: "profile2.hello.*"
        name: "profile2.hello"
        tags:
          foo: "$1"
`
	testConfig := configmock.NewFromYAML(t, datadogYaml)

	profiles, err := getDogstatsdMappingProfiles(testConfig)
	require.NoError(t, err)

	expectedProfiles := []mapper.MappingProfileConfig{
		{
			Name:   "airflow",
			Prefix: "airflow.",
			Mappings: []mapper.MetricMappingConfig{
				{
					Match:     "airflow\\.job\\.duration_sec\\.(.*)",
					MatchType: "regex",
					Name:      "airflow.job.duration",
					Tags:      map[string]string{"job_type": "$1", "job_name": "$2"},
				},
				{
					Match: "airflow.job.size.*.*",
					Name:  "airflow.job.size",
					Tags:  map[string]string{"foo": "$1", "bar": "$2"},
				},
			},
		},
		{
			Name:   "profile2",
			Prefix: "profile2.",
			Mappings: []mapper.MetricMappingConfig{
				{
					Match: "profile2.hello.*",
					Name:  "profile2.hello",
					Tags:  map[string]string{"foo": "$1"},
				},
			},
		},
	}
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesEmpty(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
`
	testConfig := configmock.NewFromYAML(t, datadogYaml)

	profiles, err := getDogstatsdMappingProfiles(testConfig)

	var expectedProfiles []mapper.MappingProfileConfig

	assert.NoError(t, err)
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesError(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - abc
`
	testConfig := configmock.NewFromYAML(t, datadogYaml)

	profiles, err := getDogstatsdMappingProfiles(testConfig)

	expectedErrorMsg := "Could not parse dogstatsd_mapper_profiles"
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), expectedErrorMsg)
	assert.Empty(t, profiles)
}

func TestDogstatsdMappingProfilesEnv(t *testing.T) {
	env := "DD_DOGSTATSD_MAPPER_PROFILES"
	t.Setenv(env, `[
{"name":"another_profile","prefix":"abcd","mappings":[
	{
		"match":"airflow\\.dag_processing\\.last_runtime\\.(.*)",
		"match_type":"regex","name":"foo",
		"tags":{"a":"$1","b":"$2"}
	}]},
{"name":"some_other_profile","prefix":"some_other_profile.","mappings":[{"match":"some_other_profile.*","name":"some_other_profile.abc","tags":{"a":"$1"}}]}
]`)
	expected := []mapper.MappingProfileConfig{
		{Name: "another_profile", Prefix: "abcd", Mappings: []mapper.MetricMappingConfig{
			{Match: "airflow\\.dag_processing\\.last_runtime\\.(.*)", MatchType: "regex", Name: "foo", Tags: map[string]string{"a": "$1", "b": "$2"}},
		}},
		{Name: "some_other_profile", Prefix: "some_other_profile.", Mappings: []mapper.MetricMappingConfig{
			{Match: "some_other_profile.*", Name: "some_other_profile.abc", Tags: map[string]string{"a": "$1"}},
		}},
	}
	cfg := configmock.New(t)
	mappings, _ := getDogstatsdMappingProfiles(cfg)
	assert.Equal(t, expected, mappings)
}
