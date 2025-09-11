// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func BenchmarkStatKeeperSameTX(b *testing.B) {
	cfg := config.New()
	cfg.MaxRedisStatsBuffered = 1000
	sk := NewStatsKeeper(cfg)

	sourceIP, destIP, sourcePort, destPort := generateAddresses()
	tx := generateRedisTransaction(sourceIP, destIP, sourcePort, destPort, uint8(GetCommand), "keyName", false, 500)

	eventWrapper := NewEventWrapper(&tx.Header, &tx.Key)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sk.Process(eventWrapper)
	}
}

func TestProcessRedisTransactions(t *testing.T) {
	cfg := config.New()
	cfg.MaxRedisStatsBuffered = 1000
	cfg.RedisTrackResources = true

	sk := NewStatsKeeper(cfg)
	sourceIP, destIP, sourcePort, destPort := generateAddresses()

	const numOfKeys = 10
	keyPrefix := "test-key"
	numIterationsPerErr := 2
	for i := 0; i < numOfKeys; i++ {
		keyName := keyPrefix + strconv.Itoa(i)

		for j := 0; j < 2*numIterationsPerErr; j++ {
			isErr := false
			if j%2 != 0 {
				isErr = true
			}
			latency := time.Duration(j%2+1) * time.Millisecond
			event := generateRedisTransaction(sourceIP, destIP, sourcePort, destPort, uint8(GetCommand), keyName, isErr, latency)
			tx := NewEventWrapper(&event.Header, &event.Key)
			sk.Process(tx)
		}
	}

	stats := sk.GetAndResetAllStats()
	assert.Equal(t, 0, len(sk.stats))
	assert.Equal(t, numOfKeys, len(stats))
	for key, stats := range stats {
		assert.Equal(t, keyPrefix, key.KeyName.Get()[:len(keyPrefix)])
		errors := []bool{false, true}
		for i, isErr := range errors {
			s := stats.ErrorToStats[isErr]
			require.NotNil(t, s)
			assert.Equal(t, numIterationsPerErr, s.Count)
			assert.Equal(t, float64(numIterationsPerErr), s.Latencies.GetCount())

			p50, err := s.Latencies.GetValueAtQuantile(0.5)
			assert.Nil(t, err)

			expectedLatency := float64(time.Duration(i+1) * time.Millisecond)
			acceptableError := expectedLatency * s.Latencies.IndexMapping.RelativeAccuracy()
			assert.GreaterOrEqual(t, p50, expectedLatency-acceptableError)
			assert.LessOrEqual(t, p50, expectedLatency+acceptableError)
		}
	}
}

func generateAddresses() (util.Address, util.Address, int, int) {
	srcString := "1.1.1.1"
	dstString := "2.2.2.2"
	sourceIP := util.AddressFromString(srcString)
	sourcePort := 1234
	destIP := util.AddressFromString(dstString)
	destPort := 9092

	return sourceIP, destIP, sourcePort, destPort
}

func generateRedisTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, command uint8, keyName string, isError bool, latency time.Duration) *EbpfKeyedEvent {
	var buf [128]byte
	copy(buf[:], keyName)
	keySize := len(keyName)
	latencyNS := uint64(latency)

	var event EbpfKeyedEvent

	event.Header.Tx.Request_started = 1
	event.Header.Tx.Response_last_seen = event.Header.Tx.Request_started + latencyNS
	event.Header.Tx.Is_error = isError
	event.Key.Buf = buf
	event.Key.Len = uint16(keySize)
	event.Header.Tx.Command = command
	event.Header.Tuple.Saddr_l = uint64(binary.LittleEndian.Uint32(source.Unmap().AsSlice()))
	event.Header.Tuple.Sport = uint16(sourcePort)
	event.Header.Tuple.Daddr_l = uint64(binary.LittleEndian.Uint32(dest.Unmap().AsSlice()))
	event.Header.Tuple.Dport = uint16(destPort)
	event.Header.Tuple.Metadata = 1

	return &event
}

func TestTrackResources(t *testing.T) {
	t.Run("track_resources enabled", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxRedisStatsBuffered = 1000
		cfg.RedisTrackResources = true
		sk := NewStatsKeeper(cfg)

		sourceIP, destIP, sourcePort, destPort := generateAddresses()
		tx := generateRedisTransaction(sourceIP, destIP, sourcePort, destPort, uint8(GetCommand), "my_key", false, 500)
		eventWrapper := NewEventWrapper(&tx.Header, &tx.Key)

		sk.Process(eventWrapper)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)

		// Verify key name is tracked
		for key := range stats {
			assert.Equal(t, "my_key", key.KeyName.Get())
			assert.Equal(t, GetCommand, key.Command)
		}
	})

	t.Run("track_resources disabled", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxRedisStatsBuffered = 1000
		sk := NewStatsKeeper(cfg)

		sourceIP, destIP, sourcePort, destPort := generateAddresses()
		tx := generateRedisTransaction(sourceIP, destIP, sourcePort, destPort, uint8(GetCommand), "my_key", false, 500)
		eventWrapper := NewEventWrapper(&tx.Header, &tx.Key)

		sk.Process(eventWrapper)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)

		// Verify key name is not tracked but command is
		for key := range stats {
			assert.Nil(t, key.KeyName)
			assert.Equal(t, GetCommand, key.Command)
		}
	})
}
