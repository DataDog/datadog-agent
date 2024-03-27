// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StatKeeper is a struct to hold the stats for the kafka protocol
type StatKeeper struct {
	stats      map[Key]*RequestStat
	statsMutex sync.RWMutex
	maxEntries int
	telemetry  *Telemetry

	// topicNames stores interned versions of the all topics currently stored in
	// the `StatKeeper`
	topicNames map[string]string
}

// NewStatkeeper creates a new StatKeeper
func NewStatkeeper(c *config.Config, telemetry *Telemetry) *StatKeeper {
	return &StatKeeper{
		stats:      make(map[Key]*RequestStat),
		maxEntries: c.MaxKafkaStatsBuffered,
		telemetry:  telemetry,
		topicNames: make(map[string]string),
	}
}

// Process processes the kafka transaction
func (statKeeper *StatKeeper) Process(tx *EbpfTx) {
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
	requestStats.Count += int(tx.RecordsCount())
}

// GetAndResetAllStats returns all the stats and resets the stats
func (statKeeper *StatKeeper) GetAndResetAllStats() map[Key]*RequestStat {
	statKeeper.statsMutex.RLock()
	defer statKeeper.statsMutex.RUnlock()
	ret := statKeeper.stats // No deep copy needed since `statKeeper.stats` gets reset
	statKeeper.stats = make(map[Key]*RequestStat)
	statKeeper.topicNames = make(map[string]string)
	return ret
}

func (statKeeper *StatKeeper) extractTopicName(tx *EbpfTx) string {
	// Limit tx.Topic_name_size to not exceed the actual length of tx.Topic_name
	if tx.Topic_name_size > uint16(len(tx.Topic_name)) {
		log.Debugf("Topic name size was changed from %d, to size: %d", tx.Topic_name_size, len(tx.Topic_name))
		tx.Topic_name_size = uint16(len(tx.Topic_name))
	}
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
