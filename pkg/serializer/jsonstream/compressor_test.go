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

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/require"
)

var (
	maxPayloadSizeDefault = maxPayloadSize
)

type dummyMarshaller struct {
	items  []string
	header string
	footer string
}

func resetDefaults() {
	maxPayloadSize = maxPayloadSizeDefault
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

func (d *dummyMarshaller) JSONFooter() []byte {
	return []byte(d.footer)
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
	c, err := newCompressor([]byte("{["), []byte("]}"))
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

	payloads, err := Payloads(m)
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
	maxPayloadSize = 22
	defer resetDefaults()

	payloads, err := Payloads(m)
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
	maxPayloadSize = 22
	defer resetDefaults()

	payloads, err := Payloads(m)
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
	payloads, err := Payloads(testSeries)
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
