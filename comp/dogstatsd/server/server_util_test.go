// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"

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
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
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

// Returns a server that is not started along with associated dependencies
// Be careful when using this functionality, as server start instantiates many internal components to non-nil values
func fulfillDepsWithInactiveServer(t *testing.T, cfg map[string]interface{}) (depsWithoutServer, *server) {
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

	return deps, s
}

type batcherMock struct {
	serviceChecks []*servicecheck.ServiceCheck
	events        []*event.Event
	lateSamples   []metrics.MetricSample
	samples       []metrics.MetricSample
}

func (b *batcherMock) appendServiceCheck(serviceCheck *servicecheck.ServiceCheck) {
	b.serviceChecks = append(b.serviceChecks, serviceCheck)
}

func (b *batcherMock) appendEvent(event *event.Event) {
	b.events = append(b.events, event)
}

func (b *batcherMock) appendLateSample(sample metrics.MetricSample) {
	b.lateSamples = append(b.lateSamples, sample)
}

func (b *batcherMock) appendSample(sample metrics.MetricSample) {
	b.samples = append(b.samples, sample)
}

func (b *batcherMock) flush() {}

func (b *batcherMock) clear() {
	b.serviceChecks = b.serviceChecks[0:0]
	b.events = b.events[0:0]
	b.lateSamples = b.lateSamples[0:0]
	b.samples = b.samples[0:0]
}

func genTestPackets(inputs ...[]byte) []*packets.Packet {
	packetSet := make([]*packets.Packet, len(inputs))
	for idx, input := range inputs {
		packet := &packets.Packet{
			Contents:   input,
			Origin:     "test-origin",
			ListenerID: "noop-listener",
			Source:     packets.UDP,
		}
		packetSet[idx] = packet
	}

	return packetSet
}

var defaultMetricInput = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

func defaultMetric() *tMetricSample {
	return &tMetricSample{
		Name:       "daemon",
		Value:      666.0,
		SampleRate: 1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"sometag1:somevalue1", "sometag2:somevalue2"},
	}
}

var defaultServiceInput = []byte("_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2")

func defaultServiceCheck() tServiceCheck {
	return tServiceCheck{
		CheckName: "agent.up",
		Host:      "localhost",
		Message:   "this is fine",
		Tags:      []string{"sometag1:somevalyyue1", "sometag2:somevalue2"},
		Status:    0,
		Ts:        12345,
	}
}

var defaultEventInput = []byte("_e{10,10}:test title|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test")

func defaultEvent() tEvent {
	return tEvent{
		Title:     "test title",
		Text:      "test\ntext",
		Tags:      []string{"tag1", "tag2:test"},
		Host:      "some.host",
		Ts:        12345,
		AlertType: event.AlertTypeWarning,

		Priority:       event.PriorityLow,
		AggregationKey: "aggKey",
		SourceTypeName: "source test",
	}
}

type tMetricSample struct {
	Name       string
	Value      float64
	Tags       []string
	Mtype      metrics.MetricType
	SampleRate float64
	RawValue   string
	Timestamp  float64
}

func (m tMetricSample) testMetric(t *testing.T, actual metrics.MetricSample) {
	s := "metric %s was expected to match"
	assert.Equal(t, m.Name, actual.Name, s, "name")
	assert.Equal(t, m.Value, actual.Value, s, "value")
	assert.Equal(t, m.Mtype, actual.Mtype, s, "type")
	assert.ElementsMatch(t, m.Tags, actual.Tags, s, "tags")
	assert.Equal(t, m.SampleRate, actual.SampleRate, s, "sample rate")
	assert.Equal(t, m.RawValue, actual.RawValue, s, "raw value")
	assert.Equal(t, m.Timestamp, actual.Timestamp, s, "timestamp")
}

func (m *tMetricSample) withName(n string) *tMetricSample {
	m.Name = n
	return m
}

func (m *tMetricSample) withValue(v float64) *tMetricSample {
	m.Value = v
	return m
}

func (m *tMetricSample) withType(t metrics.MetricType) *tMetricSample {
	m.Mtype = t
	return m
}

func (m *tMetricSample) withTags(tags []string) *tMetricSample {
	m.Tags = tags
	return m
}

func (m *tMetricSample) withSampleRate(srate float64) *tMetricSample {
	m.SampleRate = srate
	return m
}

func (m *tMetricSample) withRawValue(rval string) *tMetricSample {
	m.RawValue = rval
	return m
}

func (m *tMetricSample) withTimestamp(timestamp float64) *tMetricSample {
	m.Timestamp = timestamp
	return m
}

type tServiceCheck struct {
	CheckName string
	Host      string
	Message   string
	Ts        int64
	Tags      []string
	Status    servicecheck.ServiceCheckStatus
}

func (expected tServiceCheck) testService(t *testing.T, actual *servicecheck.ServiceCheck) {
	s := "service check %s was expected to match"
	assert.Equal(t, expected.CheckName, actual.CheckName, s, "check name")
	assert.Equal(t, expected.Host, actual.Host, s, "host")
	assert.Equal(t, expected.Message, actual.Message, s, "message")
	assert.Equal(t, expected.Ts, actual.Ts, s, "timestamp")
	assert.ElementsMatch(t, expected.Tags, actual.Tags, s, "tags")
	assert.Equal(t, expected.Status, actual.Status, s, "status")
}

type tEvent struct {
	Title          string
	Text           string
	Tags           []string
	Host           string
	Ts             int64
	AlertType      event.AlertType
	EventType      string
	Priority       event.Priority
	AggregationKey string
	SourceTypeName string
}

func (expected tEvent) testEvent(t *testing.T, actual *event.Event) {
	s := "event %s was expected to match"
	assert.Equal(t, expected.Title, actual.Title, s, "title")
	assert.Equal(t, expected.Text, actual.Text, s, "text")
	assert.ElementsMatch(t, expected.Tags, actual.Tags, s, "tags")
	assert.Equal(t, expected.Host, actual.Host, s, "host")
	assert.Equal(t, expected.Ts, actual.Ts, s, "timestamp")
	assert.Equal(t, expected.AlertType, actual.AlertType, s, "alert type")
	assert.Equal(t, expected.EventType, actual.EventType, s, "type")
	assert.Equal(t, expected.Priority, actual.Priority, s, "priority")
	assert.Equal(t, expected.AggregationKey, actual.AggregationKey, s, "aggregation key")
	assert.Equal(t, expected.SourceTypeName, actual.SourceTypeName, s, "source type name")
}
