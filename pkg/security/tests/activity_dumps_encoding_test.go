// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
	"github.com/stretchr/testify/assert"
)

//go:embed testdata/adv1.msgp
var v1testdata []byte

func getTestDataActivityDump(tb testing.TB) *probe.ActivityDump {
	ad := probe.NewEmptyActivityDump()
	if err := ad.DecodeFromReader(bytes.NewReader(v1testdata), dump.MSGP); err != nil {
		tb.Fatal(err)
	}
	return ad
}

func runEncoding(b *testing.B, encode func(ad *probe.ActivityDump) (*bytes.Buffer, error)) {
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

func BenchmarkMsgpackEncoding(b *testing.B) {
	runEncoding(b, func(ad *probe.ActivityDump) (*bytes.Buffer, error) {
		return ad.EncodeMSGP()
	})
}

func BenchmarkProtobufEncoding(b *testing.B) {
	runEncoding(b, func(ad *probe.ActivityDump) (*bytes.Buffer, error) {
		return ad.EncodeProtobuf()
	})
}

func BenchmarkProtoJSONEncoding(b *testing.B) {
	runEncoding(b, func(ad *probe.ActivityDump) (*bytes.Buffer, error) {
		return ad.EncodeProtoJSON()
	})
}

func TestProtobufDecoding(t *testing.T) {
	ad := getTestDataActivityDump(t)

	out, err := ad.EncodeProtobuf()
	if err != nil {
		t.Fatal(err)
	}

	decoded := probe.NewEmptyActivityDump()
	if err := decoded.DecodeProtobuf(bytes.NewReader(out.Bytes())); err != nil {
		t.Fatal(err)
	}

	newOut, err := ad.EncodeProtobuf()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, out.Len(), newOut.Len())
}
