// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kafka

import (
	"errors"

	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	TopicName      *intern.StringValue
	types.ConnectionKey
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
	// this field order is intentional to help the GC pointer tracking
	Latencies *ddsketch.DDSketch
	// Note: every time we add a latency value to the DDSketch, it's possible for the sketch to discard that value
	// (ie if it is outside the range that is tracked by the sketch). For that reason, in order to keep an accurate count
	// the number of kafka transactions processed, we have our own count field (rather than relying on DDSketch.GetCount())
	Count int
	// This field holds the value (in nanoseconds) of the first HTTP request
	// in this bucket. We do this as optimization to avoid creating sketches with
	// a single value. This is quite common in the context of HTTP requests without
	// keep-alives where a short-lived TCP connection is used for a single request.
	FirstLatencySample float64
	StaticTags         uint64
}

func (r *RequestStat) initSketch() error {
	latencies := protocols.SketchesPool.Get()
	if latencies == nil {
		return errors.New("error recording kafka transaction latency: could not create new ddsketch")
	}
	r.Latencies = latencies
	return nil
}

func (r *RequestStat) close() {
	if r.Latencies != nil {
		r.Latencies.Clear()
		protocols.SketchesPool.Put(r.Latencies)
		r.Latencies = nil
	}
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStats) CombineWith(newStats *RequestStats) {
	for statusCode, newRequests := range newStats.ErrorCodeToStat {
		if newRequests.Count == 0 {
			// Nothing to do in this case
			continue
		}

		if newRequests.Latencies == nil {
			// In this case, newRequests must have only FirstLatencySample, so use it when adding the request
			r.AddRequest(statusCode, newRequests.Count, newRequests.StaticTags, newRequests.FirstLatencySample)
			continue
		}

		stats, exists := r.ErrorCodeToStat[statusCode]
		if !exists {
			stats = &RequestStat{}
			r.ErrorCodeToStat[statusCode] = stats
		}
		// The other bucket (newStats) has a DDSketch object
		// We first ensure that the bucket we're merging to have a DDSketch object
		if stats.Latencies == nil {
			var err error
			stats.Latencies, err = protocols.NewSketch()
			if err != nil {
				log.Warnf("could not create new ddsketch for kafka request stats: %v", err)
				// If we can't create a new sketch, we just skip this bucket
				continue
			}
			// If the record was already present, we need to add the FirstLatencySample
			if exists {
				if stats.FirstLatencySample == 0 || stats.Count != 1 {
					log.Warnf("unexpected state for kafka request stats: FirstLatencySample=%f, Count=%d", stats.FirstLatencySample, stats.Count)
					continue
				}

				if err := stats.Latencies.Add(stats.FirstLatencySample); err != nil {
					log.Warnf("could not add kafka request latency to ddsketch: %v", err)
					continue
				}
			}
		}

		if err := stats.Latencies.MergeWith(newRequests.Latencies); err != nil {
			log.Debugf("error merging kafka transactions: %v", err)
		}
		stats.StaticTags |= newRequests.StaticTags
		stats.Count += newRequests.Count
	}
}

// AddRequest takes information about a Kafka transaction and adds it to the request stats
func (r *RequestStats) AddRequest(errorCode int32, count int, staticTags uint64, latency float64) {
	if !isValidKafkaErrorCode(errorCode) {
		return
	}
	stats, exists := r.ErrorCodeToStat[errorCode]
	if !exists {
		stats = &RequestStat{}
		r.ErrorCodeToStat[errorCode] = stats
	}
	originalCount := stats.Count
	stats.Count += count
	stats.StaticTags |= staticTags

	if stats.Count == 1 {
		stats.FirstLatencySample = latency
		return
	}

	if stats.Latencies == nil {
		if err := stats.initSketch(); err != nil {
			log.Warnf("could not add request latency to ddsketch: %v", err)
			return
		}

		// The kafka kernel decoder can capture multiple requests in a single packet, so
		// in case of a new event with multiple requests, we might not have a FirstLatencySample
		// In such a case, we need to skip adding the latency sample to the sketch
		if originalCount == 1 {
			// Add the deferred latency sample
			if err := stats.Latencies.Add(stats.FirstLatencySample); err != nil {
				log.Debugf("could not add request latency to ddsketch: %v", err)
			}
		}
	}
	if err := stats.Latencies.AddWithCount(latency, float64(count)); err != nil {
		log.Debugf("could not add request latency to ddsketch: %v", err)
	}
}

func isValidKafkaErrorCode(errorCode int32) bool {
	return errorCode >= -1 && errorCode <= 119
}

// Close releases internal stats resources.
func (r *RequestStats) Close() {
	for _, stats := range r.ErrorCodeToStat {
		if stats != nil {
			stats.close()
		}
	}
	r.ErrorCodeToStat = nil
}
