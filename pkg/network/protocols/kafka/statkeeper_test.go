// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"strings"
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

func TestStatKeeper_extractTopicName(t *testing.T) {
	type fields struct {
		topicNames map[string]string
	}
	type args struct {
		tx *EbpfTx
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "slice bigger then Topic_name",
			fields: fields{
				topicNames: map[string]string{},
			},
			args: args{&EbpfTx{
				Tup:             ConnTuple{},
				Topic_name:      [80]byte{},
				Topic_name_size: 85,
				Pad_cgo_0:       [2]byte{},
			},
			},
			want: strings.Repeat("*", 80),
		},
		{
			name: "slice smaller then Topic_name",
			fields: fields{
				topicNames: map[string]string{},
			},
			args: args{&EbpfTx{
				Tup:             ConnTuple{},
				Topic_name:      [80]byte{},
				Topic_name_size: 60,
				Pad_cgo_0:       [2]byte{},
			},
			},
			want: strings.Repeat("*", 60),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statKeeper := &StatKeeper{
				topicNames: tt.fields.topicNames,
			}
			if got := statKeeper.extractTopicName(tt.args.tx); len(got) != len(tt.want) {
				t.Errorf("extractTopicName() = %v, want %v", got, tt.want)
			}
		})
	}
}
