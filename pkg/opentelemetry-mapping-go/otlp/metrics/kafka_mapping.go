// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package metrics provides metric mappings.
package metrics

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pmetric"
)

// kafkaMetricsToRename is a map of kafka metrics that should be renamed.
var kafkaMetricsToRename = map[string]bool{
	"kafka.producer.request-rate":        true,
	"kafka.producer.response-rate":       true,
	"kafka.producer.request-latency-avg": true,
	"kafka.consumer.fetch-size-avg":      true,
	"kafka.producer.compression-rate":    true,
	"kafka.producer.record-retry-rate":   true,
	"kafka.producer.record-send-rate":    true,
	"kafka.producer.record-error-rate":   true,
}

// Note: `-` get converted into `_` which will result in some OTel and DD metric
// having the same name. In order to prevent duplicate stats, prepend by `otel.`
// in these cases.
func remapKafkaMetrics(all pmetric.MetricSlice, m pmetric.Metric) {
	name := m.Name()
	if !strings.HasPrefix(name, "kafka.") {
		// not a kafka metric
		return
	}
	switch name {
	// OOTB Kafka Dashboard
	case "kafka.producer.request-rate":
		copyMetricWithAttr(all, m, "kafka.producer.request_rate", 1,
			attributesMapping{
				fixed: map[string]string{"type": "producer-metrics"},
			},
		)
	case "kafka.producer.response-rate":
		copyMetricWithAttr(all, m, "kafka.producer.response_rate", 1,
			attributesMapping{
				fixed: map[string]string{"type": "producer-metrics"},
			},
		)
	case "kafka.producer.request-latency-avg":
		copyMetricWithAttr(all, m, "kafka.producer.request_latency_avg", 1,
			attributesMapping{
				fixed: map[string]string{"type": "producer-metrics"},
			},
		)
	case "kafka.producer.outgoing-byte-rate":
		copyMetricWithAttr(all, m, "kafka.producer.bytes_out", 1,
			attributesMapping{
				fixed: map[string]string{"type": "producer-metrics"},
			},
		)
	case "kafka.producer.io-wait-time-ns-avg":
		copyMetricWithAttr(all, m, "kafka.producer.io_wait", 1,
			attributesMapping{
				fixed: map[string]string{"type": "producer-metrics"},
			},
		)
	case "kafka.producer.byte-rate":
		copyMetricWithAttr(all, m, "kafka.producer.bytes_out", 1,
			attributesMapping{
				fixed:   map[string]string{"type": "producer-topic-metrics"},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	case "kafka.consumer.total.bytes-consumed-rate":
		copyMetricWithAttr(all, m, "kafka.consumer.bytes_in", 1,
			attributesMapping{
				fixed: map[string]string{"type": "consumer-fetch-manager-metrics"},
			},
		)
	case "kafka.consumer.total.records-consumed-rate":
		copyMetricWithAttr(all, m, "kafka.consumer.messages_in", 1,
			attributesMapping{
				fixed: map[string]string{"type": "consumer-fetch-manager-metrics"},
			},
		)
	case "kafka.network.io":
		copyMetricWithAttr(all, m, "kafka.net.bytes_out.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "BrokerTopicMetrics",
					"name": "BytesOutPerSec",
				},
			},
			kv{"state", "out"},
		)
		copyMetricWithAttr(all, m, "kafka.net.bytes_in.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "BrokerTopicMetrics",
					"name": "BytesInPerSec",
				}},
			kv{"state", "in"},
		)
	case "kafka.purgatory.size":
		copyMetricWithAttr(all, m, "kafka.request.producer_request_purgatory.size", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":             "DelayedOperationPurgatory",
					"name":             "PurgatorySize",
					"delayedOperation": "Produce",
				},
			},
			kv{"type", "produce"},
		)
		copyMetricWithAttr(all, m, "kafka.request.fetch_request_purgatory.size", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":             "DelayedOperationPurgatory",
					"name":             "PurgatorySize",
					"delayedOperation": "Fetch",
				},
			},
			kv{"type", "fetch"},
		)
	case "kafka.partition.under_replicated":
		copyMetricWithAttr(all, m, "kafka.replication.under_replicated_partitions", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "ReplicaManager",
					"name": "UnderReplicatedPartitions",
				},
			},
		)
	case "kafka.isr.operation.count":
		copyMetricWithAttr(all, m, "kafka.replication.isr_shrinks.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "ReplicaManager",
					"name": "IsrShrinksPerSec",
				},
			},
			kv{"operation", "shrink"},
		)
		copyMetricWithAttr(all, m, "kafka.replication.isr_expands.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "ReplicaManager",
					"name": "IsrExpandsPerSec",
				},
			},
			kv{"operation", "expand"},
		)
	case "kafka.leader.election.rate":
		copyMetricWithAttr(all, m, "kafka.replication.leader_elections.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "ControllerStats",
					"name": "LeaderElectionRateAndTimeMs",
				},
			},
		)
	case "kafka.partition.offline":
		copyMetricWithAttr(all, m, "kafka.replication.offline_partitions_count", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "KafkaController",
					"name": "OfflinePartitionsCount",
				},
			},
		)
	case "kafka.request.time.avg":
		copyMetricWithAttr(all, m, "kafka.request.produce.time.avg", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "Produce",
				},
			},
			kv{"type", "produce"},
		)
		copyMetricWithAttr(all, m, "kafka.request.fetch_consumer.time.avg", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchConsumer",
				},
			},
			kv{"type", "fetchconsumer"},
		)
		copyMetricWithAttr(all, m, "kafka.request.fetch_follower.time.avg", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchFollower",
				},
			},
			kv{"type", "fetchfollower"},
		)
	// non-dashboard metrics
	case "kafka.message.count":
		copyMetricWithAttr(all, m, "kafka.messages_in.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "BrokerTopicMetrics",
					"name": "MessagesInPerSec",
				},
			},
		)
	case "kafka.request.failed":
		copyMetricWithAttr(all, m, "kafka.request.produce.failed.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "BrokerTopicMetrics",
					"name": "FailedProduceRequestsPerSec",
				},
			},
			kv{"type", "produce"},
		)
		copyMetricWithAttr(all, m, "kafka.request.fetch.failed.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "BrokerTopicMetrics",
					"name": "FailedFetchRequestsPerSec",
				}},
			kv{"type", "fetch"},
		)
	case "kafka.request.time.99p":
		copyMetricWithAttr(all, m, "kafka.request.produce.time.99percentile", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "Produce",
				},
			},
			kv{"type", "produce"},
		)
		copyMetricWithAttr(all, m, "kafka.request.fetch_consumer.time.99percentile", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchConsumer",
				},
			},
			kv{"type", "fetchconsumer"},
		)
		copyMetricWithAttr(all, m, "kafka.request.fetch_follower.time.99percentile", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchFollower",
				},
			},
			kv{"type", "fetchfollower"},
		)
	case "kafka.partition.count":
		copyMetricWithAttr(all, m, "kafka.replication.partition_count", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "ReplicaManager",
					"name": "PartitionCount",
				},
			},
		)
	case "kafka.max.lag":
		copyMetricWithAttr(all, m, "kafka.replication.max_lag", 1,
			attributesMapping{
				fixed: map[string]string{
					"type":     "ReplicaFetcherManager",
					"name":     "MaxLag",
					"clientId": "replica",
				},
			},
		)
	case "kafka.controller.active.count":
		copyMetricWithAttr(all, m, "kafka.replication.active_controller_count", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "KafkaController",
					"name": "ActiveControllerCount",
				},
			},
		)
	case "kafka.unclean.election.rate":
		copyMetricWithAttr(all, m, "kafka.replication.unclean_leader_elections.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "ControllerStats",
					"name": "UncleanLeaderElectionsPerSec",
				},
			},
		)
	case "kafka.request.queue":
		copyMetricWithAttr(all, m, "kafka.request.channel.queue.size", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "RequestChannel",
					"name": "RequestQueueSize",
				},
			},
		)
	case "kafka.logs.flush.time.count":
		copyMetricWithAttr(all, m, "kafka.log.flush_rate.rate", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "LogFlushStats",
					"name": "LogFlushRateAndTimeMs",
				},
			},
		)
	case "kafka.consumer.bytes-consumed-rate":
		copyMetricWithAttr(all, m, "kafka.consumer.bytes_consumed", 1,
			attributesMapping{
				fixed:   map[string]string{"type": "consumer-fetch-manager-metrics"},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	case "kafka.consumer.records-consumed-rate":
		copyMetricWithAttr(all, m, "kafka.consumer.records_consumed", 1,
			attributesMapping{
				fixed: map[string]string{
					"type": "consumer-fetch-manager-metrics",
				},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	case "kafka.consumer.fetch-size-avg":
		copyMetricWithAttr(all, m, "kafka.consumer.fetch_size_avg", 1,
			attributesMapping{
				fixed:   map[string]string{"type": "consumer-fetch-manager-metrics"},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	case "kafka.producer.compression-rate":
		copyMetricWithAttr(all, m, "kafka.producer.compression_rate", 1,
			attributesMapping{
				fixed:   map[string]string{"type": "producer-topic-metrics"},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	case "kafka.producer.record-error-rate":
		copyMetricWithAttr(all, m, "kafka.producer.record_error_rate", 1,
			attributesMapping{
				fixed:   map[string]string{"type": "producer-topic-metrics"},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	case "kafka.producer.record-retry-rate":
		copyMetricWithAttr(all, m, "kafka.producer.record_retry_rate", 1,
			attributesMapping{
				fixed:   map[string]string{"type": "producer-topic-metrics"},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	case "kafka.producer.record-send-rate":
		copyMetricWithAttr(all, m, "kafka.producer.record_send_rate", 1,
			attributesMapping{
				fixed:   map[string]string{"type": "producer-topic-metrics"},
				dynamic: map[string]string{"client-id": "client"},
			},
		)
	// kafka metrics receiver
	case "kafka.partition.current_offset":
		copyMetricWithAttr(all, m, "kafka.broker_offset", 1,
			attributesMapping{
				dynamic: map[string]string{"group": "consumer_group"},
			},
		)
	case "kafka.consumer_group.lag":
		copyMetricWithAttr(all, m, "kafka.consumer_lag", 1,
			attributesMapping{
				dynamic: map[string]string{"group": "consumer_group"},
			},
		)
	case "kafka.consumer_group.offset":
		copyMetricWithAttr(all, m, "kafka.consumer_offset", 1,
			attributesMapping{
				dynamic: map[string]string{"group": "consumer_group"},
			},
		)
	}
}

func remapJvmMetrics(all pmetric.MetricSlice, m pmetric.Metric) {
	name := m.Name()
	if !strings.HasPrefix(name, "jvm.") {
		// not a jvm metric
		return
	}
	switch name {
	case "jvm.gc.collections.count":
		// Young Gen Collectors
		copyMetricWithAttr(all, m, "jvm.gc.minor_collection_count", 1,
			attributesMapping{
				fixed: map[string]string{"type": "GarbageCollector"},
			},
			kv{"name", "Copy"},
			kv{"name", "PS Scavenge"},
			kv{"name", "ParNew"},
			kv{"name", "G1 Young Generation"},
		)
		// Old Gen Collectors
		copyMetricWithAttr(all, m, "jvm.gc.major_collection_count", 1,
			attributesMapping{
				fixed: map[string]string{"type": "GarbageCollector"},
			},
			kv{"name", "MarkSweepCompact"},
			kv{"name", "PS MarkSweep"},
			kv{"name", "ConcurrentMarkSweep"},
			kv{"name", "G1 Mixed Generation"},
			kv{"name", "G1 Old Generation"},
			kv{"name", "Shenandoah Cycles"},
			kv{"name", "ZGC"},
		)

	case "jvm.gc.collections.elapsed":
		// Young Gen Collectors
		copyMetricWithAttr(all, m, "jvm.gc.minor_collection_time", 1,
			attributesMapping{
				fixed: map[string]string{"type": "GarbageCollector"},
			},
			kv{"name", "Copy"},
			kv{"name", "PS Scavenge"},
			kv{"name", "ParNew"},
			kv{"name", "G1 Young Generation"},
		)
		// Old Gen Collectors
		copyMetricWithAttr(all, m, "jvm.gc.major_collection_time", 1,
			attributesMapping{
				fixed: map[string]string{"type": "GarbageCollector"},
			},
			kv{"name", "MarkSweepCompact"},
			kv{"name", "PS MarkSweep"},
			kv{"name", "ConcurrentMarkSweep"},
			kv{"name", "G1 Mixed Generation"},
			kv{"name", "G1 Old Generation"},
			kv{"name", "Shenandoah Cycles"},
			kv{"name", "ZGC"},
		)
	}
}

// renameKafkaMetrics renames otel kafka metrics to avoid conflicts with DD metrics.
func renameKafkaMetrics(m pmetric.Metric) {
	if _, ok := kafkaMetricsToRename[m.Name()]; ok {
		m.SetName("otel." + m.Name())
	}
}
