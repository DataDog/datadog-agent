// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"strconv"
	"strings"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest/pmetrictest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// testPoint is a datapoint
type testPoint struct {
	// i defines a IntValue datapoint when non-zero
	i int64
	// f defines a DoubleValue datapoint when non-zero
	f float64
	// attrs specifies the raw attributes of the datapoint
	attrs map[string]any
}

// testMetric is a convenience function to create a new metric with the given name
// and set of datapoints
func testMetric(name string, dps ...testPoint) pmetric.Metric {
	out := pmetric.NewMetric()
	out.SetName(name)
	g := out.SetEmptyGauge()
	for _, d := range dps {
		p := g.DataPoints().AppendEmpty()
		if d.i != 0 {
			p.SetIntValue(d.i)
		} else {
			p.SetDoubleValue(d.f)
		}
		p.Attributes().FromRaw(d.attrs)
	}
	return out
}

func TestRemapAndRenameMetrics(t *testing.T) {
	chunit := func(m pmetric.Metric, typ string) pmetric.Metric {
		m.SetUnit(typ)
		return m
	}

	dest := pmetric.NewMetricSlice()
	for _, tt := range []struct {
		in  pmetric.Metric
		out []pmetric.Metric
	}{
		{
			in:  testMetric("system.cpu.load_average.1m", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("system.load.1", testPoint{f: 1})},
		},
		{
			in:  testMetric("system.cpu.load_average.5m", testPoint{f: 5}),
			out: []pmetric.Metric{testMetric("system.load.5", testPoint{f: 5})},
		},
		{
			in:  testMetric("system.cpu.load_average.15m", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("system.load.15", testPoint{f: 15})},
		},
		{
			in: testMetric("system.cpu.utilization",
				testPoint{f: 1, attrs: map[string]any{"state": "idle"}},
				testPoint{f: 2, attrs: map[string]any{"state": "user"}},
				testPoint{f: 3, attrs: map[string]any{"state": "system"}},
				testPoint{f: 5, attrs: map[string]any{"state": "wait"}},
				testPoint{f: 8, attrs: map[string]any{"state": "steal"}},
				testPoint{f: 13, attrs: map[string]any{"state": "other"}},
			),
			out: []pmetric.Metric{
				testMetric("system.cpu.idle",
					testPoint{f: 100, attrs: map[string]any{"state": "idle"}}),
				testMetric("system.cpu.user",
					testPoint{f: 200, attrs: map[string]any{"state": "user"}}),
				testMetric("system.cpu.system",
					testPoint{f: 300, attrs: map[string]any{"state": "system"}}),
				testMetric("system.cpu.iowait",
					testPoint{f: 500, attrs: map[string]any{"state": "wait"}}),
				testMetric("system.cpu.stolen",
					testPoint{f: 800, attrs: map[string]any{"state": "steal"}}),
			},
		},
		{
			in:  testMetric("system.cpu.utilization", testPoint{i: 5, attrs: map[string]any{"state": "idle"}}),
			out: []pmetric.Metric{testMetric("system.cpu.idle", testPoint{i: 5, attrs: map[string]any{"state": "idle"}})},
		},
		{
			in: testMetric("system.memory.usage",
				testPoint{f: divMebibytes * 1, attrs: map[string]any{"state": "free"}},
				testPoint{f: divMebibytes * 2, attrs: map[string]any{"state": "cached"}},
				testPoint{f: divMebibytes * 3, attrs: map[string]any{"state": "buffered"}},
				testPoint{f: divMebibytes * 5, attrs: map[string]any{"state": "steal"}},
				testPoint{f: divMebibytes * 8, attrs: map[string]any{"state": "system"}},
				testPoint{f: divMebibytes * 13, attrs: map[string]any{"state": "user"}},
			),
			out: []pmetric.Metric{
				testMetric("system.mem.total",
					testPoint{f: 1, attrs: map[string]any{"state": "free"}},
					testPoint{f: 2, attrs: map[string]any{"state": "cached"}},
					testPoint{f: 3, attrs: map[string]any{"state": "buffered"}},
					testPoint{f: 5, attrs: map[string]any{"state": "steal"}},
					testPoint{f: 8, attrs: map[string]any{"state": "system"}},
					testPoint{f: 13, attrs: map[string]any{"state": "user"}},
				),
				testMetric("system.mem.usable",
					testPoint{f: 1, attrs: map[string]any{"state": "free"}},
					testPoint{f: 2, attrs: map[string]any{"state": "cached"}},
					testPoint{f: 3, attrs: map[string]any{"state": "buffered"}},
				),
			},
		},
		{
			in:  testMetric("system.memory.usage", testPoint{i: divMebibytes * 5}),
			out: []pmetric.Metric{testMetric("system.mem.total", testPoint{i: 5})},
		},
		{
			in: testMetric("system.network.io",
				testPoint{f: 1, attrs: map[string]any{"direction": "receive"}},
				testPoint{f: 2, attrs: map[string]any{"direction": "transmit"}},
				testPoint{f: 3, attrs: map[string]any{"state": "buffered"}},
			),
			out: []pmetric.Metric{
				testMetric("system.net.bytes_rcvd",
					testPoint{f: 1, attrs: map[string]any{"direction": "receive"}},
				),
				testMetric("system.net.bytes_sent",
					testPoint{f: 2, attrs: map[string]any{"direction": "transmit"}},
				),
			},
		},
		{
			in: testMetric("system.paging.usage",
				testPoint{f: divMebibytes * 1, attrs: map[string]any{"state": "free"}},
				testPoint{f: divMebibytes * 2, attrs: map[string]any{"state": "used"}},
				testPoint{f: 3, attrs: map[string]any{"state": "buffered"}},
			),
			out: []pmetric.Metric{
				testMetric("system.swap.free",
					testPoint{f: 1, attrs: map[string]any{"state": "free"}},
				),
				testMetric("system.swap.used",
					testPoint{f: 2, attrs: map[string]any{"state": "used"}},
				),
			},
		},
		{
			in:  testMetric("system.filesystem.utilization", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("system.disk.in_use", testPoint{f: 15})},
		},
		{
			in:  testMetric("other.metric", testPoint{f: 15}),
			out: []pmetric.Metric{},
		},
		{
			in: testMetric("container.cpu.usage.total", testPoint{f: 15}),
			out: []pmetric.Metric{
				chunit(testMetric("container.cpu.usage", testPoint{f: 15}), "nanocore"),
			},
		},
		{
			in: testMetric("container.cpu.usage.usermode", testPoint{f: 15}),
			out: []pmetric.Metric{
				chunit(testMetric("container.cpu.user", testPoint{f: 15}), "nanocore"),
			},
		},
		{
			in: testMetric("container.cpu.usage.system", testPoint{f: 15}),
			out: []pmetric.Metric{
				chunit(testMetric("container.cpu.system", testPoint{f: 15}), "nanocore"),
			},
		},
		{
			in:  testMetric("container.cpu.throttling_data.throttled_time", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.cpu.throttled", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.cpu.throttling_data.throttled_periods", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.cpu.throttled.periods", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.memory.usage.total", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.memory.usage", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.memory.active_anon", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.memory.kernel", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.memory.hierarchical_memory_limit", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.memory.limit", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.memory.usage.limit", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.memory.soft_limit", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.memory.total_cache", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.memory.cache", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.memory.total_swap", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.memory.swap", testPoint{f: 15})},
		},
		{
			in: testMetric("container.blockio.io_service_bytes_recursive",
				testPoint{f: 1, attrs: map[string]any{"operation": "write"}},
				testPoint{f: 2, attrs: map[string]any{"operation": "read"}},
				testPoint{f: 3, attrs: map[string]any{"state": "buffered"}},
			),
			out: []pmetric.Metric{
				testMetric("container.io.write",
					testPoint{f: 1, attrs: map[string]any{"operation": "write"}}),
				testMetric("container.io.read",
					testPoint{f: 2, attrs: map[string]any{"operation": "read"}}),
			},
		},
		{
			in: testMetric("container.blockio.io_service_bytes_recursive",
				testPoint{f: 1, attrs: map[string]any{"operation": "write"}},
			),
			out: []pmetric.Metric{
				testMetric("container.io.write",
					testPoint{f: 1, attrs: map[string]any{"operation": "write"}}),
			},
		},
		{
			in: testMetric("container.blockio.io_serviced_recursive",
				testPoint{f: 1, attrs: map[string]any{"operation": "write"}},
				testPoint{f: 2, attrs: map[string]any{"operation": "read"}},
				testPoint{f: 3, attrs: map[string]any{"state": "buffered"}},
			),
			out: []pmetric.Metric{
				testMetric("container.io.write.operations",
					testPoint{f: 1, attrs: map[string]any{"operation": "write"}}),
				testMetric("container.io.read.operations",
					testPoint{f: 2, attrs: map[string]any{"operation": "read"}}),
			},
		},
		{
			in: testMetric("container.blockio.io_serviced_recursive",
				testPoint{f: 1, attrs: map[string]any{"xoperation": "write"}},
				testPoint{f: 2, attrs: map[string]any{"xoperation": "read"}},
				testPoint{f: 3, attrs: map[string]any{"state": "buffered"}},
			),
			out: nil, // no datapoints match filter
		},
		{
			in:  testMetric("container.network.io.usage.tx_bytes", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.net.sent", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.network.io.usage.tx_packets", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.net.sent.packets", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.network.io.usage.rx_bytes", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.net.rcvd", testPoint{f: 15})},
		},
		{
			in:  testMetric("container.network.io.usage.rx_packets", testPoint{f: 15}),
			out: []pmetric.Metric{testMetric("container.net.rcvd.packets", testPoint{f: 15})},
		},

		// kafka
		{
			in:  testMetric("kafka.producer.request-rate", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.producer.request_rate", testPoint{f: 1, attrs: map[string]any{"type": "producer-metrics"}})},
		},
		{
			in:  testMetric("kafka.producer.response-rate", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.producer.response_rate", testPoint{f: 1, attrs: map[string]any{"type": "producer-metrics"}})},
		},
		{
			in:  testMetric("kafka.producer.request-latency-avg", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.producer.request_latency_avg", testPoint{f: 1, attrs: map[string]any{"type": "producer-metrics"}})},
		},
		{
			in:  testMetric("kafka.producer.outgoing-byte-rate", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.producer.bytes_out", testPoint{f: 1, attrs: map[string]any{"type": "producer-metrics"}})},
		},
		{
			in:  testMetric("kafka.producer.io-wait-time-ns-avg", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.producer.io_wait", testPoint{f: 1, attrs: map[string]any{"type": "producer-metrics"}})},
		},
		{
			in: testMetric("kafka.producer.byte-rate", testPoint{f: 1, attrs: map[string]any{"client-id": "client123"}}),
			out: []pmetric.Metric{testMetric("kafka.producer.bytes_out", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
				"client":    "client123",
				"type":      "producer-topic-metrics",
			}})},
		},
		{
			in:  testMetric("kafka.consumer.total.bytes-consumed-rate", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.consumer.bytes_in", testPoint{f: 1, attrs: map[string]any{"type": "consumer-fetch-manager-metrics"}})},
		},
		{
			in:  testMetric("kafka.consumer.total.records-consumed-rate", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.consumer.messages_in", testPoint{f: 1, attrs: map[string]any{"type": "consumer-fetch-manager-metrics"}})},
		},
		{
			in: testMetric("kafka.network.io",
				testPoint{f: 1, attrs: map[string]any{
					"state": "out",
				}},
				testPoint{f: 2, attrs: map[string]any{
					"state": "in",
				}},
			),
			out: []pmetric.Metric{
				testMetric("kafka.net.bytes_out.rate", testPoint{f: 1, attrs: map[string]any{
					"type":  "BrokerTopicMetrics",
					"name":  "BytesOutPerSec",
					"state": "out",
				}}),
				testMetric("kafka.net.bytes_in.rate", testPoint{f: 2, attrs: map[string]any{
					"type":  "BrokerTopicMetrics",
					"name":  "BytesInPerSec",
					"state": "in",
				}}),
			},
		},
		{
			in: testMetric("kafka.purgatory.size",
				testPoint{f: 1, attrs: map[string]any{
					"type": "produce",
				}},
				testPoint{f: 2, attrs: map[string]any{
					"type": "fetch",
				}},
			),
			out: []pmetric.Metric{
				testMetric("kafka.request.producer_request_purgatory.size", testPoint{f: 1, attrs: map[string]any{
					"type":             "DelayedOperationPurgatory",
					"name":             "PurgatorySize",
					"delayedOperation": "Produce",
				}}),
				testMetric("kafka.request.fetch_request_purgatory.size", testPoint{f: 2, attrs: map[string]any{
					"type":             "DelayedOperationPurgatory",
					"name":             "PurgatorySize",
					"delayedOperation": "Fetch",
				}}),
			},
		},
		{
			in: testMetric("kafka.partition.under_replicated", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.replication.under_replicated_partitions", testPoint{f: 1, attrs: map[string]any{
				"type": "ReplicaManager",
				"name": "UnderReplicatedPartitions",
			}})},
		},
		{
			in: testMetric("kafka.isr.operation.count",
				testPoint{f: 1, attrs: map[string]any{
					"operation": "shrink",
				}},
				testPoint{f: 2, attrs: map[string]any{
					"operation": "expand",
				}},
			),
			out: []pmetric.Metric{
				testMetric("kafka.replication.isr_shrinks.rate", testPoint{f: 1, attrs: map[string]any{
					"type":      "ReplicaManager",
					"name":      "IsrShrinksPerSec",
					"operation": "shrink",
				}}),
				testMetric("kafka.replication.isr_expands.rate", testPoint{f: 2, attrs: map[string]any{
					"type":      "ReplicaManager",
					"name":      "IsrExpandsPerSec",
					"operation": "expand",
				}}),
			},
		},
		{
			in: testMetric("kafka.leader.election.rate", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.replication.leader_elections.rate", testPoint{f: 1, attrs: map[string]any{
				"type": "ControllerStats",
				"name": "LeaderElectionRateAndTimeMs",
			}})},
		},
		{
			in: testMetric("kafka.partition.offline", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.replication.offline_partitions_count", testPoint{f: 1, attrs: map[string]any{
				"type": "KafkaController",
				"name": "OfflinePartitionsCount",
			}})},
		},
		{
			in: testMetric("kafka.request.time.avg",
				testPoint{f: 1, attrs: map[string]any{
					"type": "produce",
				}},
				testPoint{f: 2, attrs: map[string]any{
					"type": "fetchconsumer",
				}},
				testPoint{f: 3, attrs: map[string]any{
					"type": "fetchfollower",
				}},
			),
			out: []pmetric.Metric{
				testMetric("kafka.request.produce.time.avg", testPoint{f: 1, attrs: map[string]any{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "Produce",
				}}),
				testMetric("kafka.request.fetch_consumer.time.avg", testPoint{f: 2, attrs: map[string]any{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchConsumer",
				}}),
				testMetric("kafka.request.fetch_follower.time.avg", testPoint{f: 3, attrs: map[string]any{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchFollower",
				}}),
			},
		},
		{
			in: testMetric("kafka.message.count", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.messages_in.rate", testPoint{f: 1, attrs: map[string]any{
				"type": "BrokerTopicMetrics",
				"name": "MessagesInPerSec",
			}})},
		},
		{
			in: testMetric("kafka.request.failed",
				testPoint{f: 1, attrs: map[string]any{
					"type": "produce",
				}},
				testPoint{f: 2, attrs: map[string]any{
					"type": "fetch",
				}},
			),
			out: []pmetric.Metric{
				testMetric("kafka.request.produce.failed.rate", testPoint{f: 1, attrs: map[string]any{
					"type": "BrokerTopicMetrics",
					"name": "FailedProduceRequestsPerSec",
				}}),
				testMetric("kafka.request.fetch.failed.rate", testPoint{f: 2, attrs: map[string]any{
					"type": "BrokerTopicMetrics",
					"name": "FailedFetchRequestsPerSec",
				}}),
			},
		},
		{
			in: testMetric("kafka.request.time.99p",
				testPoint{f: 1, attrs: map[string]any{
					"type": "produce",
				}},
				testPoint{f: 2, attrs: map[string]any{
					"type": "fetchconsumer",
				}},
				testPoint{f: 3, attrs: map[string]any{
					"type": "fetchfollower",
				}},
			),
			out: []pmetric.Metric{
				testMetric("kafka.request.produce.time.99percentile", testPoint{f: 1, attrs: map[string]any{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "Produce",
				}}),
				testMetric("kafka.request.fetch_consumer.time.99percentile", testPoint{f: 2, attrs: map[string]any{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchConsumer",
				}}),
				testMetric("kafka.request.fetch_follower.time.99percentile", testPoint{f: 3, attrs: map[string]any{
					"type":    "RequestMetrics",
					"name":    "TotalTimeMs",
					"request": "FetchFollower",
				}}),
			},
		},
		{
			in: testMetric("kafka.partition.count", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.replication.partition_count", testPoint{f: 1, attrs: map[string]any{
				"type": "ReplicaManager",
				"name": "PartitionCount",
			}})},
		},
		{
			in: testMetric("kafka.max.lag", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.replication.max_lag", testPoint{f: 1, attrs: map[string]any{
				"type":     "ReplicaFetcherManager",
				"name":     "MaxLag",
				"clientId": "replica",
			}})},
		},
		{
			in: testMetric("kafka.controller.active.count", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.replication.active_controller_count", testPoint{f: 1, attrs: map[string]any{
				"type": "KafkaController",
				"name": "ActiveControllerCount",
			}})},
		},
		{
			in: testMetric("kafka.unclean.election.rate", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.replication.unclean_leader_elections.rate", testPoint{f: 1, attrs: map[string]any{
				"type": "ControllerStats",
				"name": "UncleanLeaderElectionsPerSec",
			}})},
		},
		{
			in: testMetric("kafka.request.queue", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.request.channel.queue.size", testPoint{f: 1, attrs: map[string]any{
				"type": "RequestChannel",
				"name": "RequestQueueSize",
			}})},
		},
		{
			in: testMetric("kafka.logs.flush.time.count", testPoint{f: 1}),
			out: []pmetric.Metric{testMetric("kafka.log.flush_rate.rate", testPoint{f: 1, attrs: map[string]any{
				"type": "LogFlushStats",
				"name": "LogFlushRateAndTimeMs",
			}})},
		},
		{
			in: testMetric("kafka.consumer.bytes-consumed-rate", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.consumer.bytes_consumed", testPoint{f: 1, attrs: map[string]any{
				"type":      "consumer-fetch-manager-metrics",
				"client-id": "client123",
				"client":    "client123",
			}})},
		},
		{
			in: testMetric("kafka.consumer.records-consumed-rate", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.consumer.records_consumed", testPoint{f: 1, attrs: map[string]any{
				"type":      "consumer-fetch-manager-metrics",
				"client-id": "client123",
				"client":    "client123",
			}})},
		},
		{
			in: testMetric("kafka.consumer.fetch-size-avg", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.consumer.fetch_size_avg", testPoint{f: 1, attrs: map[string]any{
				"type":      "consumer-fetch-manager-metrics",
				"client-id": "client123",
				"client":    "client123",
			}})},
		},
		{
			in: testMetric("kafka.producer.compression-rate", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.producer.compression_rate", testPoint{f: 1, attrs: map[string]any{
				"type":      "producer-topic-metrics",
				"client-id": "client123",
				"client":    "client123",
			}})},
		},
		{
			in: testMetric("kafka.producer.record-error-rate", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.producer.record_error_rate", testPoint{f: 1, attrs: map[string]any{
				"type":      "producer-topic-metrics",
				"client-id": "client123",
				"client":    "client123",
			}})},
		},
		{
			in: testMetric("kafka.producer.record-retry-rate", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.producer.record_retry_rate", testPoint{f: 1, attrs: map[string]any{
				"type":      "producer-topic-metrics",
				"client-id": "client123",
				"client":    "client123",
			}})},
		},
		{
			in: testMetric("kafka.producer.record-send-rate", testPoint{f: 1, attrs: map[string]any{
				"client-id": "client123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.producer.record_send_rate", testPoint{f: 1, attrs: map[string]any{
				"type":      "producer-topic-metrics",
				"client-id": "client123",
				"client":    "client123",
			}})},
		},

		// kafka metrics receiver
		{
			in: testMetric("kafka.partition.current_offset", testPoint{f: 1, attrs: map[string]any{
				"group": "group123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.broker_offset", testPoint{f: 1, attrs: map[string]any{
				"group":          "group123",
				"consumer_group": "group123",
			}})},
		},
		{
			in: testMetric("kafka.consumer_group.lag", testPoint{f: 1, attrs: map[string]any{
				"group": "group123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.consumer_lag", testPoint{f: 1, attrs: map[string]any{
				"group":          "group123",
				"consumer_group": "group123",
			}})},
		},
		{
			in: testMetric("kafka.consumer_group.offset", testPoint{f: 1, attrs: map[string]any{
				"group": "group123",
			}}),
			out: []pmetric.Metric{testMetric("kafka.consumer_offset", testPoint{f: 1, attrs: map[string]any{
				"group":          "group123",
				"consumer_group": "group123",
			}})},
		},

		// jvm
		{
			in: testMetric("jvm.gc.collections.count",
				testPoint{f: 1, attrs: map[string]any{"name": "Copy"}},
				testPoint{f: 2, attrs: map[string]any{"name": "PS Scavenge"}},
				testPoint{f: 3, attrs: map[string]any{"name": "ParNew"}},
				testPoint{f: 4, attrs: map[string]any{"name": "G1 Young Generation"}},
			),
			out: []pmetric.Metric{
				testMetric("jvm.gc.minor_collection_count",
					testPoint{f: 1, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "Copy",
					}},
					testPoint{f: 2, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "PS Scavenge",
					}},
					testPoint{f: 3, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "ParNew",
					}},
					testPoint{f: 4, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "G1 Young Generation",
					}}),
			},
		},
		{
			in: testMetric("jvm.gc.collections.count",
				testPoint{f: 1, attrs: map[string]any{"name": "MarkSweepCompact"}},
				testPoint{f: 2, attrs: map[string]any{"name": "PS MarkSweep"}},
				testPoint{f: 3, attrs: map[string]any{"name": "ConcurrentMarkSweep"}},
				testPoint{f: 4, attrs: map[string]any{"name": "G1 Mixed Generation"}},
				testPoint{f: 5, attrs: map[string]any{"name": "G1 Old Generation"}},
				testPoint{f: 6, attrs: map[string]any{"name": "Shenandoah Cycles"}},
				testPoint{f: 7, attrs: map[string]any{"name": "ZGC"}},
			),
			out: []pmetric.Metric{
				testMetric("jvm.gc.major_collection_count",
					testPoint{f: 1, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "MarkSweepCompact",
					}},
					testPoint{f: 2, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "PS MarkSweep",
					}},
					testPoint{f: 3, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "ConcurrentMarkSweep",
					}},
					testPoint{f: 4, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "G1 Mixed Generation",
					}},
					testPoint{f: 5, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "G1 Old Generation",
					}},
					testPoint{f: 6, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "Shenandoah Cycles",
					}},
					testPoint{f: 7, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "ZGC",
					}}),
			},
		},
		{
			in: testMetric("jvm.gc.collections.elapsed",
				testPoint{f: 1, attrs: map[string]any{"name": "Copy"}},
				testPoint{f: 2, attrs: map[string]any{"name": "PS Scavenge"}},
				testPoint{f: 3, attrs: map[string]any{"name": "ParNew"}},
				testPoint{f: 4, attrs: map[string]any{"name": "G1 Young Generation"}},
			),
			out: []pmetric.Metric{
				testMetric("jvm.gc.minor_collection_time",
					testPoint{f: 1, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "Copy",
					}},
					testPoint{f: 2, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "PS Scavenge",
					}},
					testPoint{f: 3, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "ParNew",
					}},
					testPoint{f: 4, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "G1 Young Generation",
					}}),
			},
		},
		{
			in: testMetric("jvm.gc.collections.elapsed",
				testPoint{f: 1, attrs: map[string]any{"name": "MarkSweepCompact"}},
				testPoint{f: 2, attrs: map[string]any{"name": "PS MarkSweep"}},
				testPoint{f: 3, attrs: map[string]any{"name": "ConcurrentMarkSweep"}},
				testPoint{f: 4, attrs: map[string]any{"name": "G1 Mixed Generation"}},
				testPoint{f: 5, attrs: map[string]any{"name": "G1 Old Generation"}},
				testPoint{f: 6, attrs: map[string]any{"name": "Shenandoah Cycles"}},
				testPoint{f: 7, attrs: map[string]any{"name": "ZGC"}},
			),
			out: []pmetric.Metric{
				testMetric("jvm.gc.major_collection_time",
					testPoint{f: 1, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "MarkSweepCompact",
					}},
					testPoint{f: 2, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "PS MarkSweep",
					}},
					testPoint{f: 3, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "ConcurrentMarkSweep",
					}},
					testPoint{f: 4, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "G1 Mixed Generation",
					}},
					testPoint{f: 5, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "G1 Old Generation",
					}},
					testPoint{f: 6, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "Shenandoah Cycles",
					}},
					testPoint{f: 7, attrs: map[string]any{
						"type": "GarbageCollector",
						"name": "ZGC",
					}}),
			},
		},
	} {
		lena := dest.Len()
		checkprefix := strings.HasPrefix(tt.in.Name(), "system.") ||
			strings.HasPrefix(tt.in.Name(), "process.") ||

			tt.in.Name() == "kafka.producer.request-rate" ||
			tt.in.Name() == "kafka.producer.response-rate" ||
			tt.in.Name() == "kafka.producer.request-latency-avg" ||

			tt.in.Name() == "kafka.consumer.fetch-size-avg" ||
			tt.in.Name() == "kafka.producer.compression-rate" ||
			tt.in.Name() == "kafka.producer.record-error-rate" ||
			tt.in.Name() == "kafka.producer.record-retry-rate" ||
			tt.in.Name() == "kafka.producer.record-send-rate"
		remapMetrics(dest, tt.in)
		// Ensure remapMetrics does not add the otel.* prefix to the metric name
		if checkprefix {
			require.False(t, strings.HasPrefix(tt.in.Name(), "otel."), "remapMetrics should not add the otel.* prefix to the metric name, it should only compute Datadog metrics")
		}
		renameMetrics(tt.in)
		if checkprefix {
			require.True(t, strings.HasPrefix(tt.in.Name(), "otel."), "system.* and process.*  and a subset of kafka metrics need to be prepended with the otel.* namespace")
		}
		require.Equal(t, dest.Len()-lena, len(tt.out), "unexpected number of metrics added")
		for i, out := range tt.out {
			assert.NoError(t, pmetrictest.CompareMetric(out, dest.At(dest.Len()-len(tt.out)+i)))
		}
	}

}

func TestRenameAgentMetrics(t *testing.T) {
	for _, tt := range []struct {
		in           pmetric.Metric
		expectedName string
	}{
		{
			in:           testMetric("datadog_trace_agent_stats_writer_bytes", testPoint{f: 1}),
			expectedName: "otelcol_datadog_trace_agent_stats_writer_bytes",
		},
		{
			in:           testMetric("datadog_otlp_translator_resources_missing_source", testPoint{f: 1}),
			expectedName: "otelcol_datadog_otlp_translator_resources_missing_source",
		},
		{
			in:           testMetric("http_server_duration", testPoint{f: 1}),
			expectedName: "http_server_duration",
		},
		{
			in:           testMetric("http_server_request_size", testPoint{f: 1}),
			expectedName: "http_server_request_size",
		},
		{
			in:           testMetric("http_server_response_size", testPoint{f: 1}),
			expectedName: "http_server_response_size",
		},
		// Verify no duplicated prefix is added (for metrics <= 0.105.0)
		{
			in:           testMetric("otelcol_datadog_trace_agent_stats_writer_bytes", testPoint{f: 1}),
			expectedName: "otelcol_datadog_trace_agent_stats_writer_bytes",
		},
		{
			in:           testMetric("otelcol_datadog_otlp_translator_resources_missing_source", testPoint{f: 1}),
			expectedName: "otelcol_datadog_otlp_translator_resources_missing_source",
		},
	} {
		renameMetrics(tt.in)
		assert.Equal(t, tt.expectedName, tt.in.Name())
	}
}

func TestCopyMetricWithAttr(t *testing.T) {
	m := pmetric.NewMetric()
	m.SetName("test.metric")
	m.SetDescription("metric-description")
	m.SetUnit("cm")

	dest := pmetric.NewMetricSlice()
	t.Run("gauge", func(t *testing.T) {
		v := m.SetEmptyGauge()
		dp := v.DataPoints().AppendEmpty()
		dp.SetDoubleValue(12)
		dp.Attributes().FromRaw(map[string]any{"fruit": "apple", "count": 15})
		dp = v.DataPoints().AppendEmpty()
		dp.SetIntValue(24)
		dp.Attributes().FromRaw(map[string]any{"human": "Ann", "age": 25})

		t.Run("plain", func(t *testing.T) {
			out, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 1, attributesMapping{})
			require.True(t, ok)
			require.Equal(t, m.Name(), "test.metric")
			require.Equal(t, out.Name(), "copied.test.metric")
			sameExceptName(t, m, out)
			require.Equal(t, dest.At(dest.Len()-1), out)
		})

		t.Run("div", func(t *testing.T) {
			out, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 2, attributesMapping{})
			require.True(t, ok)
			require.Equal(t, out.Name(), "copied.test.metric")
			require.Equal(t, out.Gauge().DataPoints().At(0).DoubleValue(), 6.)
			require.Equal(t, out.Gauge().DataPoints().At(1).IntValue(), int64(12))
			require.Equal(t, dest.At(dest.Len()-1), out)
		})

		t.Run("filter", func(t *testing.T) {
			out, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 1, attributesMapping{}, kv{"human", "Ann"})
			require.True(t, ok)
			require.Equal(t, out.Name(), "copied.test.metric")
			require.Equal(t, out.Gauge().DataPoints().Len(), 1)
			require.Equal(t, out.Gauge().DataPoints().At(0).IntValue(), int64(24))
			require.Equal(t, out.Gauge().DataPoints().At(0).Attributes().AsRaw(), map[string]any{"human": "Ann", "age": int64(25)})
			require.Equal(t, dest.At(dest.Len()-1), out)
		})
		t.Run("attributesMapping", func(t *testing.T) {
			out, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 1, attributesMapping{
				fixed:   map[string]string{"fixed.attr": "ok"},
				dynamic: map[string]string{"fruit": "remapped_fruit"},
			})
			require.True(t, ok)
			require.Equal(t, m.Name(), "test.metric")
			require.Equal(t, out.Name(), "copied.test.metric")

			aa, bb := pmetric.NewMetric(), pmetric.NewMetric()
			m.CopyTo(aa)
			out.CopyTo(bb)

			aa.SetName("common.name")
			// add attributes mappings manually.
			aa.Gauge().DataPoints().At(0).Attributes().PutStr("fixed.attr", "ok")
			aa.Gauge().DataPoints().At(0).Attributes().PutStr("remapped_fruit", "apple")
			aa.Gauge().DataPoints().At(1).Attributes().PutStr("fixed.attr", "ok")

			bb.SetName("common.name")
			require.Equal(t, aa, bb)

			require.Equal(t, dest.At(dest.Len()-1), out)
		})
		t.Run("dynamicattrmissing", func(t *testing.T) {
			out, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 1, attributesMapping{
				dynamic: map[string]string{"nonexistingattr": "remapped_nonexistingattr"},
			})
			require.True(t, ok)
			require.Equal(t, m.Name(), "test.metric")
			require.Equal(t, out.Name(), "copied.test.metric")
			// don't add dynamic attribute if it is missing.
			sameExceptName(t, m, out)
			require.Equal(t, dest.At(dest.Len()-1), out)
		})
		t.Run("none", func(t *testing.T) {
			_, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 1, attributesMapping{}, kv{"human", "Paul"})
			require.False(t, ok)
		})
	})

	t.Run("sum", func(t *testing.T) {
		dp := m.SetEmptySum().DataPoints().AppendEmpty()
		dp.SetDoubleValue(12)
		dp.Attributes().FromRaw(map[string]any{"fruit": "apple", "count": 15})
		out, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 1, attributesMapping{})
		require.True(t, ok)
		require.Equal(t, out.Name(), "copied.test.metric")
		sameExceptName(t, m, out)
		require.Equal(t, dest.At(dest.Len()-1), out)
	})

	t.Run("histogram", func(t *testing.T) {
		dp := m.SetEmptyHistogram().DataPoints().AppendEmpty()
		dp.SetCount(12)
		dp.SetMax(44)
		dp.SetMin(3)
		dp.SetSum(120)
		_, ok := copyMetricWithAttr(dest, m, "copied.test.metric", 1, attributesMapping{})
		require.False(t, ok)
	})
}

func TestHasAny(t *testing.T) {
	// p returns a numberic data point having the given attributes.
	p := func(m map[string]any) pmetric.NumberDataPoint {
		v := pmetric.NewNumberDataPoint()
		if err := v.Attributes().FromRaw(m); err != nil {
			t.Fatalf("Error generating data point: %v", err)
		}
		return v
	}
	for i, tt := range []struct {
		attrs map[string]any
		tags  []kv
		out   bool
	}{
		{
			attrs: map[string]any{
				"fruit": "apple",
				"human": "Ann",
			},
			tags: []kv{{"human", "Ann"}},
			out:  true,
		},
		{
			attrs: map[string]any{
				"fruit": "apple",
				"human": "Ann",
			},
			tags: []kv{{"human", "ann"}},
			out:  false,
		},
		{
			attrs: map[string]any{
				"fruit":   "apple",
				"human":   "Ann",
				"company": "Paul",
			},
			tags: []kv{{"human", "ann"}, {"company", "Paul"}},
			out:  true,
		},
		{
			attrs: map[string]any{
				"fruit":   "apple",
				"human":   "Ann",
				"company": "Paul",
			},
			tags: []kv{{"fruit", "apple"}, {"company", "Paul"}},
			out:  true,
		},
		{
			attrs: map[string]any{
				"fruit":   "apple",
				"human":   "Ann",
				"company": "Paul",
			},
			tags: nil,
			out:  true,
		},
		{
			attrs: map[string]any{
				"fruit":   "apple",
				"human":   "Ann",
				"company": "Paul",
				"number":  4,
			},
			tags: []kv{{"number", "4"}},
			out:  false,
		},
		{
			attrs: nil,
			tags:  []kv{{"number", "4"}},
			out:   false,
		},
		{
			attrs: nil,
			tags:  nil,
			out:   true,
		},
	} {
		require.Equal(t, hasAny(p(tt.attrs), tt.tags...), tt.out, strconv.Itoa(i))
	}
}

// sameExceptName validates that metrics a and b are the same by disregarding
// their names.
func sameExceptName(t *testing.T, a, b pmetric.Metric) {
	aa, bb := pmetric.NewMetric(), pmetric.NewMetric()
	a.CopyTo(aa)
	b.CopyTo(bb)
	aa.SetName("🙂")
	bb.SetName("🙂")
	require.Equal(t, aa, bb)
}
