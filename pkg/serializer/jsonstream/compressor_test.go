// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//+build zlib

package jsonstream

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var (
	maxPayloadSizeDefault = config.Datadog.GetInt("serializer_max_payload_size")
)

type dummyMarshaller struct {
	items  []string
	header string
	footer string
}

func resetDefaults() {
	config.Datadog.SetDefault("serializer_max_payload_size", maxPayloadSizeDefault)
}

func (d *dummyMarshaller) JSONHeader() []byte {
	return []byte(d.header)
}

func (d *dummyMarshaller) Len() int {
	return len(d.items)
}

func (d *dummyMarshaller) JSONItem(i int) ([]byte, error) {
	if i < 0 || i > d.Len()-1 {
		return nil, errors.New("out of range")
	}
	return []byte(d.items[i]), nil
}

func (d *dummyMarshaller) DescribeItem(i int) string {
	if i < 0 || i > d.Len()-1 {
		return "out of range"
	}
	return d.items[i]
}

func (d *dummyMarshaller) JSONFooter() []byte {
	return []byte(d.footer)
}

func (d *dummyMarshaller) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *dummyMarshaller) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *dummyMarshaller) SplitPayload(int) ([]marshaler.Marshaler, error) {
	return nil, fmt.Errorf("not implemented")
}

func decompressPayload(payload []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	dst, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

func payloadToString(payload []byte) string {
	p, err := decompressPayload(payload)
	if err != nil {
		return err.Error()
	}
	return string(p)
}

func TestCompressorSimple(t *testing.T) {
	c, err := newCompressor(&bytes.Buffer{}, &bytes.Buffer{}, []byte("{["), []byte("]}"))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		c.addItem([]byte("A"))
	}

	p, err := c.close()
	require.NoError(t, err)
	require.Equal(t, "{[A,A,A,A,A]}", payloadToString(p))
}

func TestOnePayloadSimple(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C"},
		header: "{[",
		footer: "]}",
	}

	builder := NewPayloadBuilder()
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
}

func TestMaxCompressedSizePayload(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C"},
		header: "{[",
		footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewPayloadBuilder()
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
}

func TestTwoPayload(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C", "D", "E", "F"},
		header: "{[",
		footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewPayloadBuilder()
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 2)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
	require.Equal(t, "{[D,E,F]}", payloadToString(*payloads[1]))
}

// test taken from the spliter
func TestPayloadsSeries(t *testing.T) {
	testSeries := metrics.Series{}
	for i := 0; i < 30000; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
				{Ts: 2222.0, Value: float64(22.12)},
				{Ts: 333.0, Value: float64(32.12)},
				{Ts: 444444.0, Value: float64(42.12)},
				{Ts: 882787.0, Value: float64(52.12)},
				{Ts: 99990.0, Value: float64(62.12)},
				{Ts: 121212.0, Value: float64(72.12)},
				{Ts: 222227.0, Value: float64(82.12)},
				{Ts: 808080.0, Value: float64(92.12)},
				{Ts: 9090.0, Value: float64(13.12)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		}
		testSeries = append(testSeries, &point)
	}

	originalLength := len(testSeries)
	builder := NewPayloadBuilder()
	payloads, err := builder.Build(testSeries)
	require.Nil(t, err)
	var splitSeries = []metrics.Series{}
	for _, compressedPayload := range payloads {
		payload, err := decompressPayload(*compressedPayload)
		require.NoError(t, err)

		var s = map[string]metrics.Series{}
		err = json.Unmarshal(payload, &s)
		require.NoError(t, err)
		splitSeries = append(splitSeries, s["series"])
	}

	unrolledSeries := metrics.Series{}
	for _, series := range splitSeries {
		for _, s := range series {
			unrolledSeries = append(unrolledSeries, s)
		}
	}

	newLength := len(unrolledSeries)
	require.Equal(t, originalLength, newLength)
}

var result forwarder.Payloads

func BenchmarkPayloadsSeries(b *testing.B) {
	testSeries := metrics.Series{}
	for i := 0; i < 400000; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: 1.2 * float64(i)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		}
		testSeries = append(testSeries, &point)
	}

	var r forwarder.Payloads
	builder := NewPayloadBuilder()
	for n := 0; n < b.N; n++ {
		// always record the result of Payloads to prevent
		// the compiler eliminating the function call.
		r, _ = builder.Build(testSeries)
	}
	// ensure we actually had to split
	if len(r) != 2 {
		panic(fmt.Sprintf("expecting two payloads, got %d", len(r)))
	}
	// test the compressed size
	var compressedSize int
	for _, p := range r {
		if p == nil {
			continue
		}
		compressedSize += len([]byte(*p))
	}
	if compressedSize > 3000000 {
		panic(fmt.Sprintf("expecting no more than 3 MB, got %d", compressedSize))
	}
	// always store the result to a package level variable
	// so the compiler cannot eliminate the Benchmark itself.
	result = r
}
