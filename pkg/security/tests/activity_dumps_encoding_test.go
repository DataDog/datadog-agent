// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
)

//go:embed testdata/adv1.protobuf
var v1testdata []byte

func getTestDataActivityDump(tb testing.TB) *dump.ActivityDump {
	ad := dump.NewEmptyActivityDump(nil, false, 0, nil, nil)
	if err := ad.Profile.DecodeFromReader(bytes.NewReader(v1testdata), config.Protobuf); err != nil {
		tb.Fatal(err)
	}
	return ad
}

func runEncoding(b *testing.B, encode func(ad *dump.ActivityDump) (*bytes.Buffer, error)) {
	b.Helper()
	ad := getTestDataActivityDump(b)

	size := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := encode(ad)
		if err != nil {
			b.Fatal(err)
		}
		size = raw.Len()
	}
	b.ReportMetric(float64(size), "output_size")
}

func BenchmarkProtobufEncoding(b *testing.B) {
	runEncoding(b, func(ad *dump.ActivityDump) (*bytes.Buffer, error) {
		return ad.Profile.EncodeSecDumpProtobuf()
	})
}

func BenchmarkProtoJSONEncoding(b *testing.B) {
	runEncoding(b, func(ad *dump.ActivityDump) (*bytes.Buffer, error) {
		return ad.Profile.EncodeJSON("")
	})
}

func TestProtobufDecoding(t *testing.T) {
	SkipIfNotAvailable(t)

	ad := getTestDataActivityDump(t)

	out, err := ad.Profile.EncodeSecDumpProtobuf()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeAD(out)
	if err != nil {
		t.Fatal(err)
	}

	newOut, err := decoded.Profile.EncodeSecDumpProtobuf()
	if err != nil {
		t.Fatal(err)
	}

	if !assert.Equal(t, out.Len(), newOut.Len()) {
		diffActivityDumps(t, out, newOut)
	}
}

func decodeAD(buffer *bytes.Buffer) (*dump.ActivityDump, error) {
	decoded := dump.NewEmptyActivityDump(nil, false, 0, nil, nil)
	if err := decoded.Profile.DecodeSecDumpProtobuf(bytes.NewReader(buffer.Bytes())); err != nil {
		return nil, err
	}
	return decoded, nil
}

func diffActivityDumps(tb testing.TB, a, b *bytes.Buffer) {
	ad, err := decodeAD(a)
	if err != nil {
		tb.Fatal(err)
	}

	bd, err := decodeAD(b)
	if err != nil {
		tb.Fatal(err)
	}

	assert.Equal(tb, ad, bd)
}
