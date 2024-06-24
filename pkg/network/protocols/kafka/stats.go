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
	// Go uses optimized map access implementations if the key is int32/int64, so using int32 instead of int8
	// Here you can find the original CPU impact when using int8:
	// https://dd.datad0g.com/dashboard/s3s-3hu-mh6/usm-performance-evaluation-20?fromUser=true&refresh_mode=paused&tpl_var_base_agent-env%5B0%5D=kafka-error-base&tpl_var_client-service%5B0%5D=kafka-client-%2A&tpl_var_compare_agent-env%5B0%5D=kafka-error-new&tpl_var_kube_cluster_name%5B0%5D=usm-datad0g&tpl_var_server-service%5B0%5D=kafka-broker&view=spans&from_ts=1719153394917&to_ts=1719156854000&live=false
	ErrorCodeToStat map[int32]*RequestStat
}

// NewRequestStats creates a new RequestStats object.
func NewRequestStats() *RequestStats {
	return &RequestStats{
		ErrorCodeToStat: make(map[int32]*RequestStat),
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
func (r *RequestStats) AddRequest(errorCode int32, count int, staticTags uint64) {
	if !isValidKafkaErrorCode(errorCode) {
		return
	}
	if stats, exists := r.ErrorCodeToStat[errorCode]; exists {
		stats.Count += count
		stats.StaticTags |= staticTags
	} else {
		r.ErrorCodeToStat[errorCode] = &RequestStat{
			Count:      count,
			StaticTags: staticTags,
		}
	}
}

func isValidKafkaErrorCode(errorCode int32) bool {
	return errorCode >= -1 && errorCode <= 119
}
