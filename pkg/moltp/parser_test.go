// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package moltp

import (
	"testing"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type mockMetricSink struct{}

func (m *mockMetricSink) Append(s *metrics.Serie) {}

type mockSketchSink struct {
	ss []*metrics.SketchSeries
}

func (m *mockSketchSink) Append(s *metrics.SketchSeries) {
	m.ss = append(m.ss, s)
}

func TestHistogram(t *testing.T) {
	ms := &mockMetricSink{}
	ss := &mockSketchSink{}
	cx := newCtx(ms, ss)

	// slighly re-arranged snippet from tcpdump
	// contains a histogram of 10 points from 0 to 9.
	src := []byte( /*                                */ "\x0a\xb0\x03\x0a" + /* |................| */
		"\xa9\x01\x0a\x13\x0a\x09\x62\x75\x6c\x6b\x5f\x6d\x6f\x64\x65\x12" + /* |......bulk_mode.| */
		"\x06\x0a\x04\x6f\x74\x6c\x70\x0a\x26\x0a\x0c\x73\x65\x72\x76\x69" + /* |...otlp.&..servi| */
		"\x63\x65\x2e\x6e\x61\x6d\x65\x12\x16\x0a\x14\x75\x6e\x6b\x6e\x6f" + /* |ce.name....unkno| */
		"\x77\x6e\x5f\x73\x65\x72\x76\x69\x63\x65\x3a\x6a\x61\x76\x61\x0a" + /* |wn_service:java.| */
		"\x20\x0a\x16\x74\x65\x6c\x65\x6d\x65\x74\x72\x79\x2e\x73\x64\x6b" + /* | ..telemetry.sdk| */
		"\x2e\x6c\x61\x6e\x67\x75\x61\x67\x65\x12\x06\x0a\x04\x6a\x61\x76" + /* |.language....jav| */
		"\x61\x0a\x25\x0a\x12\x74\x65\x6c\x65\x6d\x65\x74\x72\x79\x2e\x73" + /* |a.%..telemetry.s| */
		"\x64\x6b\x2e\x6e\x61\x6d\x65\x12\x0f\x0a\x0d\x6f\x70\x65\x6e\x74" + /* |dk.name....opent| */
		"\x65\x6c\x65\x6d\x65\x74\x72\x79\x0a\x21\x0a\x15\x74\x65\x6c\x65" + /* |elemetry.!..tele| */
		"\x6d\x65\x74\x72\x79\x2e\x73\x64\x6b\x2e\x76\x65\x72\x73\x69\x6f" + /* |metry.sdk.versio| */
		"\x6e\x12\x08\x0a\x06\x31\x2e\x34\x36\x2e\x30\x12\x81\x02\x0a\x1f" + /* |n....1.46.0.....| */
		"\x0a\x1d\x63\x6f\x6d\x2e\x64\x61\x74\x61\x64\x6f\x67\x68\x71\x2e" + /* |..com.datadoghq.| */
		"\x64\x73\x64\x6f\x74\x6c\x70\x2e\x41\x64\x61\x70\x74\x65\x72\x12" + /* |dsdotlp.Adapter.| */
		"\xdd\x01\x0a\x15\x64\x69\x73\x74\x72\x69\x62\x75\x74\x69\x6f\x6e" + /* |....distribution| */
		"\x2e\x6d\x65\x74\x72\x69\x63\x2e\x30\x52\xc3\x01\x0a\xbe\x01\x11" + /* |.metric.0R......| */
		"\x7b\x62\x5b\xb4\x76\x7c\x2b\x18\x19\x21\xda\x66\x08\x79\x7c\x2b" + /* |{b[.v|+..!.f.y|+| */
		"\x18\x21\x0a\x00\x00\x00\x00\x00\x00\x00\x29\x00\x00\x00\x00\x00" + /* |.!........).....| */
		"\x80\x46\x40\x61\x00\x00\x00\x00\x00\x00\x00\x00\x69\x00\x00\x00" + /* |.F@a........i...| */
		"\x00\x00\x00\x22\x40\x30\x0a\x39\x01\x00\x00\x00\x00\x00\x00\x00" + /* |..."@0.9........| */
		"\x42\x6b\x08\x01\x12\x67\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00" + /* |Bk...g..........| */
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" + /* |................| */
		"\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00" + /* |................| */
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00" + /* |................| */
		"\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00" + /* |................| */
		"\x00\x01\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00" + /* |................| */
		"\x01\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x01\x4a\x00\x0a" + /* |.............J..| */
		"\x0c\x0a\x05\x74\x61\x67\x2e\x30\x12\x03\x0a\x01\x30\x10\x01" /*       |...tag.0....0...| */)

	err := exportMetricsRequest.parseBytes(cx, src)
	require.NoError(t, err)
	require.Len(t, ss.ss, 1)

	sk := ss.ss[0]
	require.Equal(t, "distribution.metric.0", sk.Name)
	require.Equal(t, []string{"tag.0:0"}, sk.Tags.UnsafeToReadOnlySliceString())
	require.Len(t, sk.Points, 1)

	sp := sk.Points[0]


	require.Equal(t, sp.Sketch.Basic.Cnt, int64(10))
	require.Equal(t, sp.Sketch.Basic.Sum, float64(45))
	require.Equal(t, sp.Sketch.Basic.Avg, float64(4.5))
	require.Equal(t, sp.Sketch.Basic.Min, float64(0))
	require.Equal(t, sp.Sketch.Basic.Max, float64(9))

	k, n := sp.Sketch.Cols()
	require.Equal(t, []int32{0, 1339, 1340, 1341, 1384, 1385, 1386, 1411, 1412, 1429, 1430, 1444, 1445, 1446, 1455, 1456, 1457, 1465, 1466, 1467, 1474, 1475, 1482, 1483}, k)
	require.Equal(t, []uint32{1, 0, 1, 0, 0, 1, 0, 1, 0, 0, 1, 0, 1, 0, 0, 1, 0, 0, 1, 0, 1, 0, 0, 1}, n)

	conf := quantile.Default()
	require.InEpsilon(t, 1, sp.Sketch.Quantile(conf, 0.1), 0.05)
	require.InEpsilon(t, 4, sp.Sketch.Quantile(conf, 0.5), 0.05)
	require.InEpsilon(t, 8, sp.Sketch.Quantile(conf, 0.9), 0.05)
}
