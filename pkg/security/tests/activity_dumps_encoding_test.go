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
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

//go:embed testdata/adv1.protobuf
var v1testdata []byte

func getTestDataProfile(tb testing.TB) *profile.Profile {
	p := profile.New()
	if err := p.DecodeFromReader(bytes.NewReader(v1testdata), config.Protobuf); err != nil {
		tb.Fatal(err)
	}
	return p
}

func runEncoding(b *testing.B, encode func(p *profile.Profile) (*bytes.Buffer, error)) {
	b.Helper()
	p := getTestDataProfile(b)

	size := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := encode(p)
		if err != nil {
			b.Fatal(err)
		}
		size = raw.Len()
	}
	b.ReportMetric(float64(size), "output_size")
}

func BenchmarkProtobufEncoding(b *testing.B) {
	runEncoding(b, func(p *profile.Profile) (*bytes.Buffer, error) {
		return p.EncodeSecDumpProtobuf()
	})
}

func BenchmarkProtoJSONEncoding(b *testing.B) {
	runEncoding(b, func(p *profile.Profile) (*bytes.Buffer, error) {
		return p.EncodeJSON("")
	})
}

func TestProtobufDecoding(t *testing.T) {
	SkipIfNotAvailable(t)

	p := getTestDataProfile(t)

	out, err := p.EncodeSecDumpProtobuf()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeProfile(out)
	if err != nil {
		t.Fatal(err)
	}

	newOut, err := decoded.EncodeSecDumpProtobuf()
	if err != nil {
		t.Fatal(err)
	}

	if !assert.Equal(t, out.Len(), newOut.Len()) {
		diffProfiles(t, out, newOut)
	}
}

func decodeProfile(buffer *bytes.Buffer) (*profile.Profile, error) {
	decoded := profile.New()
	if err := decoded.DecodeSecDumpProtobuf(bytes.NewReader(buffer.Bytes())); err != nil {
		return nil, err
	}
	return decoded, nil
}

func diffProfiles(tb testing.TB, a, b *bytes.Buffer) {
	pa, err := decodeProfile(a)
	if err != nil {
		tb.Fatal(err)
	}

	pb, err := decodeProfile(b)
	if err != nil {
		tb.Fatal(err)
	}

	assert.Equal(tb, pa, pb)
}
