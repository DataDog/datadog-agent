// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build zlib
// +build zlib

package stream

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var (
	maxPayloadSizeDefault = config.Datadog.GetInt("serializer_max_payload_size")
)

func resetDefaults() {
	config.Datadog.SetDefault("serializer_max_payload_size", maxPayloadSizeDefault)
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
	c, err := NewCompressor(&bytes.Buffer{}, &bytes.Buffer{}, []byte("{["), []byte("]}"), []byte(","))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		c.AddItem([]byte("A"))
	}

	p, err := c.Close()
	require.NoError(t, err)
	require.Equal(t, "{[A,A,A,A,A]}", payloadToString(p))
}

func TestOnePayloadSimple(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C"},
		Header: "{[",
		Footer: "]}",
	}

	builder := NewJSONPayloadBuilder(true)
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
}

func TestMaxCompressedSizePayload(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C"},
		Header: "{[",
		Footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewJSONPayloadBuilder(true)
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
}

func TestTwoPayload(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C", "D", "E", "F"},
		Header: "{[",
		Footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewJSONPayloadBuilder(true)
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 2)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
	require.Equal(t, "{[D,E,F]}", payloadToString(*payloads[1]))
}

func TestLockedCompressorProducesSamePayloads(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C", "D", "E", "F"},
		Header: "{[",
		Footer: "]}",
	}
	defer resetDefaults()

	builderLocked := NewJSONPayloadBuilder(true)
	builderUnLocked := NewJSONPayloadBuilder(false)
	payloads1, err := builderLocked.Build(m)
	require.NoError(t, err)
	payloads2, err := builderUnLocked.Build(m)
	require.NoError(t, err)

	require.Equal(t, payloadToString(*payloads1[0]), payloadToString(*payloads2[0]))
}
