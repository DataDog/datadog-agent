// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/mapper"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// This is a copy of the serverDeps struct, but without the server field.
// We need this to avoid starting multiple server with the same test.
type depsWithoutServer struct {
	fx.In

	Config        configComponent.Component
	Log           log.Component
	Demultiplexer demultiplexer.FakeSamplerMock
	Replay        replay.Component
	PidMap        pidmap.Component
	Debug         serverdebug.Component
	WMeta         optional.Option[workloadmeta.Component]
	Telemetry     telemetry.Component
}

type serverDeps struct {
	fx.In

	Config        configComponent.Component
	Log           log.Component
	Demultiplexer demultiplexer.FakeSamplerMock
	Replay        replay.Component
	PidMap        pidmap.Component
	Debug         serverdebug.Component
	WMeta         optional.Option[workloadmeta.Component]
	Telemetry     telemetry.Component
	Server        Component
}

func fulfillDeps(t testing.TB) serverDeps {
	return fulfillDepsWithConfigOverride(t, map[string]interface{}{})
}

func fulfillDepsWithConfigOverride(t testing.TB, overrides map[string]interface{}) serverDeps {
	// TODO: https://datadoghq.atlassian.net/browse/AMLII-1948
	if runtime.GOOS == "darwin" {
		flake.Mark(t)
	}
	return fxutil.Test[serverDeps](t, fx.Options(
		core.MockBundle(),
		serverdebugimpl.MockModule(),
		fx.Replace(configComponent.MockParams{
			Overrides: overrides,
		}),
		replaymock.MockModule(),
		compressionimpl.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		Module(Params{Serverless: false}),
	))
}

func fulfillDepsWithConfigYaml(t testing.TB, yaml string) serverDeps {
	return fxutil.Test[serverDeps](t, fx.Options(
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) configComponent.Component { return configComponent.NewMockFromYAML(t, yaml) }),
		telemetryimpl.MockModule(),
		hostnameimpl.MockModule(),
		serverdebugimpl.MockModule(),
		replaymock.MockModule(),
		compressionimpl.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		Module(Params{Serverless: false}),
	))
}

func TestNewServer(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	requireStart(t, deps.Server)

}

func TestStopServer(t *testing.T) {
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
	s.start(context.TODO())
	requireStart(t, s)

	s.stop(context.TODO())

	// check that the port can be bound, try for 100 ms
	address, err := net.ResolveUDPAddr("udp", s.UDPLocalAddr())
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

func TestUDPReceive(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off

	deps := fulfillDepsWithConfigOverride(t, cfg)
	demux := deps.Demultiplexer

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to UDP network")
	defer conn.Close()

	testReceive(t, conn, demux)
}

func TestUDPForward(t *testing.T) {
	cfg := make(map[string]interface{})

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	pcHost, pcPort, err := net.SplitHostPort(pc.LocalAddr().String())
	require.NoError(t, err)

	// Setup UDP server to forward to
	cfg["statsd_forward_port"] = pcPort
	cfg["statsd_forward_host"] = pcHost

	// Setup dogstatsd server
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)

	defer pc.Close()

	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	// Check if message is forwarded
	message := []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	_, err = conn.Write(message)
	require.NoError(t, err, "cannot write to DSD socket")

	_ = pc.SetReadDeadline(time.Now().Add(4 * time.Second))

	buffer := make([]byte, len(message))
	_, _, err = pc.ReadFrom(buffer)
	require.NoError(t, err)

	assert.Equal(t, message, buffer)
}

func TestHistToDist(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["histogram_copy_to_distribution"] = true
	cfg["histogram_copy_to_distribution_prefix"] = "dist."

	deps := fulfillDepsWithConfigOverride(t, cfg)

	demux := deps.Demultiplexer
	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	_, err = conn.Write([]byte("daemon:666|h|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
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

func TestExtraTags(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_tags"] = []string{"sometag3:somevalue3"}

	env.SetFeatures(t, env.EKSFargate)
	deps := fulfillDepsWithConfigOverride(t, cfg)

	demux := deps.Demultiplexer
	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	_, err = conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
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

func TestParseMetricMessageTelemetry(t *testing.T) {
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

	assert.Nil(t, s.mapper)

	var samples []metrics.MetricSample

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	assert.Equal(t, float64(0), s.tlmProcessedOk.Get())
	samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "", "", false)
	assert.NoError(t, err)
	assert.Len(t, samples, 1)
	assert.Equal(t, float64(1), s.tlmProcessedOk.Get())

	assert.Equal(t, float64(0), s.tlmProcessedError.Get())
	samples, err = s.parseMetricMessage(samples, parser, nil, "", "", false)
	assert.Error(t, err, "invalid dogstatsd message format")
	assert.Len(t, samples, 1)
	assert.Equal(t, float64(1), s.tlmProcessedError.Get())
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
dogstatsd_port: __random__
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
dogstatsd_port: __random__
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
dogstatsd_port: __random__
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
			deps := fulfillDepsWithConfigYaml(t, scenario.config)

			s := deps.Server.(*server)

			requireStart(t, s)

			assert.Equal(t, deps.Config.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize, "Case `%s` failed. cache_size `%s` should be `%s`", scenario.name, deps.Config.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize)

			var actualSamples []MetricSample
			for _, p := range scenario.packets {
				parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
				samples, err := s.parseMetricMessage(samples, parser, []byte(p), "", "", false)
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
		})
	}
}

func TestParseEventMessageTelemetry(t *testing.T) {
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

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	telemetryMock, ok := deps.Telemetry.(telemetry.Mock)
	assert.True(t, ok)

	// three successful events
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	// one error event
	_, err := s.parseEventMessage(parser, nil, "")
	assert.Error(t, err)

	processedEvents, err := telemetryMock.GetCountMetric("dogstatsd", "processed")
	require.NoError(t, err)

	for _, metric := range processedEvents {
		labels := metric.Tags()

		if labels["message_type"] == "events" && labels["state"] == "ok" {
			assert.Equal(t, float64(3), metric.Value())
		}

		if labels["message_type"] == "events" && labels["state"] == "error" {
			assert.Equal(t, float64(1), metric.Value())
		}
	}
}

func TestParseServiceCheckMessageTelemetry(t *testing.T) {
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

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	telemetryMock, ok := deps.Telemetry.(telemetry.Mock)
	assert.True(t, ok)

	// three successful events
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	// one error event
	_, err := s.parseServiceCheckMessage(parser, nil, "")
	assert.Error(t, err)

	processedEvents, err := telemetryMock.GetCountMetric("dogstatsd", "processed")
	require.NoError(t, err)

	for _, metric := range processedEvents {
		labels := metric.Tags()

		if labels["message_type"] == "service_checks" && labels["state"] == "ok" {
			assert.Equal(t, float64(3), metric.Value())
		}

		if labels["message_type"] == "service_checks" && labels["state"] == "error" {
			assert.Equal(t, float64(1), metric.Value())
		}
	}
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

func TestProcessedMetricsOrigin(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		cfg := make(map[string]interface{})
		cfg["dogstatsd_origin_optout_enabled"] = enabled
		cfg["dogstatsd_port"] = listeners.RandomPortName

		deps := fulfillDepsWithConfigOverride(t, cfg)
		s := deps.Server.(*server)
		assert := assert.New(t)

		s.Stop()

		assert.Len(s.cachedOriginCounters, 0, "this cache must be empty")
		assert.Len(s.cachedOrder, 0, "this cache list must be empty")

		parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
		samples := []metrics.MetricSample{}
		samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "test_container", "1", false)
		assert.NoError(err)
		assert.Len(samples, 1)

		// one thing should have been stored when we parse a metric
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:555|g"), "test_container", "1", true)
		assert.NoError(err)
		assert.Len(samples, 2)
		assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
		assert.Len(s.cachedOrder, 1, "one entry should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")

		// when we parse another metric (different value) with same origin, cache should contain only one entry
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "test_container", "2", true)
		assert.NoError(err)
		assert.Len(samples, 3)
		assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
		assert.Len(s.cachedOrder, 1, "one entry should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "test_container"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "test_container"})

		// when we parse another metric (different value) but with a different origin, we should store a new entry
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "another_container", "3", true)
		assert.NoError(err)
		assert.Len(samples, 4)
		assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached")
		assert.Len(s.cachedOrder, 2, "two entries should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "test_container"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "test_container"})
		assert.Equal(s.cachedOrder[1].origin, "another_container")
		assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "another_container"})
		assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "another_container"})

		// oldest one should be removed once we reach the limit of the cache
		maxOriginCounters = 2
		samples, err = s.parseMetricMessage(samples, parser, []byte("yetanothermetric:525|g"), "third_origin", "3", true)
		assert.NoError(err)
		assert.Len(samples, 5)
		assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached, one has been evicted already")
		assert.Len(s.cachedOrder, 2, "two entries should have been cached, one has been evicted already")
		assert.Equal(s.cachedOrder[0].origin, "another_container")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "another_container"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "another_container"})
		assert.Equal(s.cachedOrder[1].origin, "third_origin")
		assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "third_origin"})
		assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "third_origin"})

		// oldest one should be removed once we reach the limit of the cache
		maxOriginCounters = 2
		samples, err = s.parseMetricMessage(samples, parser, []byte("blablabla:555|g"), "fourth_origin", "4", true)
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
