// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	ddstatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

type testStatsProcessor struct {
	last                *pb.ClientStatsPayload
	lang, ver, cid, obf string
}

func (t *testStatsProcessor) ProcessStats(ctx context.Context, p *pb.ClientStatsPayload, lang, tracerVersion, containerID, obfuscationVersion string) error {
	t.last, t.lang, t.ver, t.cid, t.obf = p, lang, tracerVersion, containerID, obfuscationVersion
	return nil
}

func TestBypassReceiver_SubmitTraces_MsgpackV07(t *testing.T) {
	conf := config.New()
	conf.Endpoints[0].APIKey = "test"
	out := make(chan *Payload, 1)
	br := NewBypassReceiver(conf, out, &testStatsProcessor{}, &statsdNoop{})

	// Build a minimal V07 TracerPayload msgpack
	tp := &pb.TracerPayload{Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "svc", Name: "op"}}}}}
	b, err := tp.MarshalMsg(nil)
	assert.NoError(t, err)
	err = br.SubmitTraces(context.Background(), V07, map[string]string{
		"Content-Type":                "application/msgpack",
		"Datadog-Meta-Lang":           "go",
		"Datadog-Meta-Lang-Version":   "1.22",
		"Datadog-Meta-Tracer-Version": "x.y.z",
	}, b)
	assert.NoError(t, err)

	select {
	case p := <-out:
		assert.Equal(t, "svc", p.TracerPayload.Chunks[0].Spans[0].Service)
		assert.Equal(t, "go", p.Source.Lang)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for payload")
	}
}

func TestBypassReceiver_SubmitStats(t *testing.T) {
	conf := config.New()
	conf.Endpoints[0].APIKey = "test"
	out := make(chan *Payload, 1)
	sp := &testStatsProcessor{}
	br := NewBypassReceiver(conf, out, sp, &statsdNoop{})

	// Build a minimal ClientStatsPayload msgpack
	in := &pb.ClientStatsPayload{}
	buf, err := in.MarshalMsg(nil)
	assert.NoError(t, err)
	err = br.SubmitStats(context.Background(), map[string]string{
		"Datadog-Meta-Lang":           "python",
		"Datadog-Meta-Tracer-Version": "v1",
	}, buf)
	assert.NoError(t, err)
	assert.NotNil(t, sp.last)
	assert.Equal(t, "python", sp.lang)
}

// statsdNoop implements statsd.ClientInterface minimally for tests.
type statsdNoop struct{}

func (*statsdNoop) Count(string, int64, []string, float64) error                         { return nil }
func (*statsdNoop) CountWithTimestamp(string, int64, []string, float64, time.Time) error { return nil }
func (*statsdNoop) Decr(string, []string, float64) error                                 { return nil }
func (*statsdNoop) Distribution(string, float64, []string, float64) error                { return nil }
func (*statsdNoop) Event(*ddstatsd.Event) error                                          { return nil }
func (*statsdNoop) Gauge(string, float64, []string, float64) error                       { return nil }
func (*statsdNoop) GaugeWithTimestamp(string, float64, []string, float64, time.Time) error {
	return nil
}
func (*statsdNoop) Histogram(string, float64, []string, float64) error           { return nil }
func (*statsdNoop) Incr(string, []string, float64) error                         { return nil }
func (*statsdNoop) Set(string, string, []string, float64) error                  { return nil }
func (*statsdNoop) SetWriteTimeout(time.Duration) error                          { return nil }
func (*statsdNoop) SimpleEvent(string, string) error                             { return nil }
func (*statsdNoop) TimeInMilliseconds(string, float64, []string, float64) error  { return nil }
func (*statsdNoop) Timing(string, time.Duration, []string, float64) error        { return nil }
func (*statsdNoop) Close() error                                                 { return nil }
func (*statsdNoop) Flush() error                                                 { return nil }
func (*statsdNoop) IsClosed() bool                                               { return false }
func (*statsdNoop) GetTelemetry() ddstatsd.Telemetry                             { return ddstatsd.Telemetry{} }
func (*statsdNoop) ServiceCheck(_ *ddstatsd.ServiceCheck) error                  { return nil }
func (*statsdNoop) SimpleServiceCheck(string, ddstatsd.ServiceCheckStatus) error { return nil }

// msgp plumbing to avoid unused import
var _ = msgp.WrapError
var _ = info.NewReceiverStats
var _ timing.Reporter = (*timing.NoopReporter)(nil)
