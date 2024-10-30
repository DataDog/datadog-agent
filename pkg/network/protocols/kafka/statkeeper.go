// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StatKeeper is a struct to hold the stats for the kafka protocol
type StatKeeper struct {
	stats      map[Key]*RequestStats
	statsMutex sync.RWMutex
	maxEntries int
	telemetry  *Telemetry

	// topicNames stores interned versions of the all topics currently stored in
	// the `StatKeeper`
	topicNames *intern.StringInterner
}

// NewStatkeeper creates a new StatKeeper
func NewStatkeeper(c *config.Config, telemetry *Telemetry) *StatKeeper {
	return &StatKeeper{
		stats:      make(map[Key]*RequestStats),
		maxEntries: c.MaxKafkaStatsBuffered,
		telemetry:  telemetry,
		topicNames: intern.NewStringInterner(),
	}
}

// Process processes the kafka transaction
func (statKeeper *StatKeeper) Process(tx *EbpfTx) {
	latency := tx.RequestLatency()
	// Produce requests with acks = 0 do not receive a response, and as a result, have no latency
	if tx.APIKey() == FetchAPIKey && latency <= 0 {
		statKeeper.telemetry.invalidLatency.Add(int64(tx.RecordsCount()))
		return
	}

	// extractTopicName is an expensive operation but, it is also concurrent safe, so we can do it here
	// without holding the lock.
	key := Key{
		RequestAPIKey:  tx.APIKey(),
		RequestVersion: tx.APIVersion(),
		TopicName:      statKeeper.extractTopicName(&tx.Transaction),
		ConnectionKey:  tx.ConnTuple(),
	}

	statKeeper.statsMutex.Lock()
	defer statKeeper.statsMutex.Unlock()
	requestStats, ok := statKeeper.stats[key]
	if !ok {
		if len(statKeeper.stats) >= statKeeper.maxEntries {
			statKeeper.telemetry.dropped.Add(int64(tx.RecordsCount()))
			return
		}
		requestStats = NewRequestStats()
		statKeeper.stats[key] = requestStats
	}

	requestStats.AddRequest(int32(tx.ErrorCode()), int(tx.RecordsCount()), uint64(tx.Transaction.Tags), latency)
}

// GetAndResetAllStats returns all the stats and resets the stats
func (statKeeper *StatKeeper) GetAndResetAllStats() map[Key]*RequestStats {
	statKeeper.statsMutex.RLock()
	defer statKeeper.statsMutex.RUnlock()
	ret := statKeeper.stats // No deep copy needed since `statKeeper.stats` gets reset
	statKeeper.stats = make(map[Key]*RequestStats)
	return ret
}

func (statKeeper *StatKeeper) extractTopicName(tx *KafkaTransaction) *intern.StringValue {
	// Limit tx.Topic_name_size to not exceed the actual length of tx.Topic_name
	if uint16(tx.Topic_name_size) > uint16(len(tx.Topic_name)) {
		log.Debugf("Topic name size was changed from %d, to size: %d", tx.Topic_name_size, len(tx.Topic_name))
		tx.Topic_name_size = uint8(len(tx.Topic_name))
	}
	b := tx.Topic_name[:tx.Topic_name_size]

	return statKeeper.topicNames.Get(b)
}
