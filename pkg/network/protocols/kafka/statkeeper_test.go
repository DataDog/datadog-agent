// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"encoding/binary"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func BenchmarkStatKeeperSameTX(b *testing.B) {
	cfg := &config.Config{MaxKafkaStatsBuffered: 1000}
	tel := NewTelemetry()
	sk := NewStatkeeper(cfg, tel)

	topicName := []byte("foobar")
	topicNameSize := len(topicName)

	tx := new(KafkaTransaction)
	copy(tx.Topic_name[:], topicName)
	tx.Topic_name_size = uint8(topicNameSize)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sk.Process(&EbpfTx{Transaction: *tx})
	}
}

func TestStatKeeper_extractTopicName(t *testing.T) {
	tests := []struct {
		name string
		tx   *KafkaTransaction
		want string
	}{
		{
			name: "slice bigger then Topic_name",
			tx: &KafkaTransaction{
				Topic_name:      [80]byte{},
				Topic_name_size: 85,
			},
			want: strings.Repeat("*", 80),
		},
		{
			name: "slice smaller then Topic_name",
			tx: &KafkaTransaction{
				Topic_name:      [80]byte{},
				Topic_name_size: 60,
			},
			want: strings.Repeat("*", 60),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statKeeper := &StatKeeper{
				topicNames: map[string]string{},
			}
			copy(tt.tx.Topic_name[:], strings.Repeat("*", len(tt.tx.Topic_name)))
			if got := statKeeper.extractTopicName(tt.tx); len(got) != len(tt.want) {
				t.Errorf("extractTopicName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessKafkaTransactions(t *testing.T) {
	cfg := &config.Config{MaxKafkaStatsBuffered: 1000}
	tel := NewTelemetry()
	sk := NewStatkeeper(cfg, tel)

	srcString := "1.1.1.1"
	dstString := "2.2.2.2"
	sourceIP := util.AddressFromString(srcString)
	sourcePort := 1234
	destIP := util.AddressFromString(dstString)
	destPort := 9092

	const numOfTopics = 10
	topicNamePrefix := "test-topic"
	for i := 0; i < numOfTopics; i++ {
		topicName := topicNamePrefix + strconv.Itoa(i)

		for j := 0; j < 10; j++ {
			errorCode := j % 5
			latency := time.Duration(j%5+1) * time.Millisecond
			tx := generateKafkaTransaction(sourceIP, destIP, sourcePort, destPort, topicName, int8(errorCode), uint32(10), latency)
			sk.Process(tx)
		}
	}

	stats := sk.GetAndResetAllStats()
	assert.Equal(t, 0, len(sk.stats))
	assert.Equal(t, numOfTopics, len(stats))
	for key, stats := range stats {
		assert.Equal(t, topicNamePrefix, key.TopicName[:len(topicNamePrefix)])
		for i := 0; i < 5; i++ {
			s := stats.ErrorCodeToStat[int32(i)]
			require.NotNil(t, s)
			assert.Equal(t, 20, s.Count)
			assert.Equal(t, 20.0, s.Latencies.GetCount())

			p50, err := s.Latencies.GetValueAtQuantile(0.5)
			assert.Nil(t, err)

			expectedLatency := float64(time.Duration(i+1) * time.Millisecond)
			acceptableError := expectedLatency * s.Latencies.IndexMapping.RelativeAccuracy()
			assert.GreaterOrEqual(t, p50, expectedLatency-acceptableError)
			assert.LessOrEqual(t, p50, expectedLatency+acceptableError)
		}
	}
}

func generateKafkaTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, topicName string, errorCode int8, recordsCount uint32, latency time.Duration) *EbpfTx {
	var event EbpfTx

	latencyNS := uint64(latency)
	event.Transaction.Request_started = 1
	event.Transaction.Request_api_key = FetchAPIKey
	event.Transaction.Request_api_version = 7
	event.Transaction.Response_last_seen = event.Transaction.Request_started + latencyNS
	event.Transaction.Error_code = errorCode
	event.Transaction.Records_count = recordsCount
	event.Transaction.Topic_name_size = uint8(len(topicName))
	event.Transaction.Topic_name = topicNameFromString([]byte(topicName))
	event.Tup.Saddr_l = uint64(binary.LittleEndian.Uint32(source.Bytes()))
	event.Tup.Sport = uint16(sourcePort)
	event.Tup.Daddr_l = uint64(binary.LittleEndian.Uint32(dest.Bytes()))
	event.Tup.Dport = uint16(destPort)
	event.Tup.Metadata = 1

	return &event
}

func topicNameFromString(fragment []byte) [TopicNameMaxSize]byte {
	if len(fragment) >= TopicNameMaxSize {
		return *(*[TopicNameMaxSize]byte)(fragment)
	}
	var b [TopicNameMaxSize]byte
	copy(b[:], fragment)
	return b
}
