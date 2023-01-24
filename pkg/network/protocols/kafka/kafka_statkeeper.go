// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

type kafkaStatKeeper struct {
	stats      map[Key]*RequestStats
	maxEntries int
	telemetry  *telemetry
}

func newKafkaStatkeeper(c *config.Config, telemetry *telemetry) *kafkaStatKeeper {
	return &kafkaStatKeeper{
		stats:      make(map[Key]*RequestStats),
		maxEntries: c.MaxHTTPStatsBuffered,
		telemetry:  telemetry,
	}
}

func (statKeeper *kafkaStatKeeper) Process(tx *ebpfKafkaTx) {
	statKeeper.add(tx)
}

func (statKeeper *kafkaStatKeeper) add(transaction *ebpfKafkaTx) {
	key := Key{
		KeyTuple: KeyTuple{
			SrcIPHigh: transaction.SrcIPHigh(),
			SrcIPLow:  transaction.SrcIPLow(),
			SrcPort:   transaction.SrcPort(),
			DstIPHigh: transaction.DstIPHigh(),
			DstIPLow:  transaction.DstIPLow(),
			DstPort:   transaction.DstPort(),
		},
		TopicName: transaction.TopicName(),
	}
	requestStats, ok := statKeeper.stats[key]
	if !ok {
		if len(statKeeper.stats) >= statKeeper.maxEntries {
			statKeeper.telemetry.dropped.Add(1)
			return
		}
		requestStats = new(RequestStats)
		statKeeper.stats[key] = requestStats
	}
	// Need to create both stats so either of them will not be nil at the end of the process
	if requestStats.Data[ProduceAPIKey] == nil {
		requestStats.Data[ProduceAPIKey] = new(RequestStat)
	}
	if requestStats.Data[FetchAPIKey] == nil {
		requestStats.Data[FetchAPIKey] = new(RequestStat)
	}
	requestStats.Data[transaction.APIKey()].Count++
}

func (statKeeper *kafkaStatKeeper) GetAndResetAllStats() map[Key]*RequestStats {
	ret := statKeeper.stats // No deep copy needed since `statKeeper.stats` gets reset
	statKeeper.stats = make(map[Key]*RequestStats)
	return ret
}
