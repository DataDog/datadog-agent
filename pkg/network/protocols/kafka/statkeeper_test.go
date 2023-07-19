// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func BenchmarkStatKeeperSameTX(b *testing.B) {
	cfg := &config.Config{MaxKafkaStatsBuffered: 1000}
	tel := NewTelemetry()
	sk := NewStatkeeper(cfg, tel)

	topicName := []byte("foobar")
	topicNameSize := len(topicName)

	tx := new(EbpfTx)
	copy(tx.Topic_name[:], topicName)
	tx.Topic_name_size = uint16(topicNameSize)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sk.Process(tx)
	}
}
