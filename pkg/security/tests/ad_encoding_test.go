// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

func getTestDataActivityDump(tb testing.TB) *probe.ActivityDump {
	ad := &probe.ActivityDump{}
	if err := ad.Decode("./pkg/security/adproto/ad_testdata.msgp"); err != nil {
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
