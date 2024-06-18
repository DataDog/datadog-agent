// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kafka

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	// ProduceAPIKey is the API key for produce requests
	ProduceAPIKey = 0

	// FetchAPIKey is the API key for fetch requests
	FetchAPIKey = 1
)

// Key is an identifier for a group of Kafka transactions
type Key struct {
	RequestAPIKey  uint16
	RequestVersion uint16
	TopicName      string
	types.ConnectionKey
}

// NewKey generates a new Key
func NewKey(saddr, daddr util.Address, sport, dport uint16, topicName string, requestAPIKey, requestAPIVersion uint16) Key {
	return Key{
		ConnectionKey:  types.NewConnectionKey(saddr, daddr, sport, dport),
		TopicName:      topicName,
		RequestAPIKey:  requestAPIKey,
		RequestVersion: requestAPIVersion,
	}
}

// RequestStats stores Kafka request statistics per Kafka error code
// We include the error code here and not in the Key to avoid creating a new Key for each error code
type RequestStats struct {
	ErrorCodeToStat map[int8]*RequestStat
}

// NewRequestStats creates a new RequestStats object.
func NewRequestStats() *RequestStats {
	return &RequestStats{
		ErrorCodeToStat: make(map[int8]*RequestStat),
	}
}

// RequestStat stores stats for Kafka requests to a particular key
type RequestStat struct {
	Count      int
	StaticTags uint64
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStats) CombineWith(newStats *RequestStats) {
	for statusCode, newRequests := range newStats.ErrorCodeToStat {
		if newRequests.Count == 0 {
			// Nothing to do in this case
			continue
		}
		r.AddRequest(statusCode, newRequests.Count, newRequests.StaticTags)
	}
}

// AddRequest takes information about a Kafka transaction and adds it to the request stats
func (r *RequestStats) AddRequest(errorCode int8, count int, staticTags uint64) {
	if !isValidKafkaErrorCode(errorCode) {
		return
	}
	stats, exists := r.ErrorCodeToStat[errorCode]
	if !exists {
		stats = &RequestStat{}
		r.ErrorCodeToStat[errorCode] = stats
	}
	stats.Count += count
	stats.StaticTags |= staticTags
}

func isValidKafkaErrorCode(errorCode int8) bool {
	return errorCode >= -1 && errorCode <= 119
}
