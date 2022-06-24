// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
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

func BenchmarkMsgpackEncoding(b *testing.B) {
	ad := getTestDataActivityDump(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ad.EncodeMSGP()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtobufEncoding(b *testing.B) {
	ad := getTestDataActivityDump(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ad.EncodeProtobuf()
		if err != nil {
			b.Fatal(err)
		}
	}
}
