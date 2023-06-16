// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

type KafkaStatKeeper struct {
	stats      map[Key]*RequestStat
	statsMutex sync.RWMutex
	maxEntries int
	telemetry  *Telemetry

	// topicNames stores interned versions of the all topics currently stored in
	// the `KafkaStatKeeper`
	topicNames map[string]string
}

func NewKafkaStatkeeper(c *config.Config, telemetry *Telemetry) *KafkaStatKeeper {
	return &KafkaStatKeeper{
		stats:      make(map[Key]*RequestStat),
		maxEntries: c.MaxKafkaStatsBuffered,
		telemetry:  telemetry,
		topicNames: make(map[string]string),
	}
}

func (statKeeper *KafkaStatKeeper) Process(tx *EbpfKafkaTx) {
	statKeeper.statsMutex.Lock()
	defer statKeeper.statsMutex.Unlock()

	key := Key{
		RequestAPIKey:  tx.APIKey(),
		RequestVersion: tx.APIVersion(),
		TopicName:      statKeeper.extractTopicName(tx),
		ConnectionKey:  tx.ConnTuple(),
	}
	requestStats, ok := statKeeper.stats[key]
	if !ok {
		if len(statKeeper.stats) >= statKeeper.maxEntries {
			statKeeper.telemetry.dropped.Add(1)
			return
		}
		requestStats = new(RequestStat)
		statKeeper.stats[key] = requestStats
	}
	requestStats.Count++
}

func (statKeeper *KafkaStatKeeper) GetAndResetAllStats() map[Key]*RequestStat {
	statKeeper.statsMutex.RLock()
	defer statKeeper.statsMutex.RUnlock()
	ret := statKeeper.stats // No deep copy needed since `statKeeper.stats` gets reset
	statKeeper.stats = make(map[Key]*RequestStat)
	statKeeper.topicNames = make(map[string]string)
	return ret
}

func (statKeeper *KafkaStatKeeper) extractTopicName(tx *EbpfKafkaTx) string {
	b := tx.Topic_name[:tx.Topic_name_size]

	// the trick here is that the Go runtime doesn't allocate the string used in
	// the map lookup, so if we have seen this topic name before, we don't
	// perform any allocations
	if v, ok := statKeeper.topicNames[string(b)]; ok {
		return v
	}

	v := string(b)
	statKeeper.topicNames[v] = v
	return v
}
