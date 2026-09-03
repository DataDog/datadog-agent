// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

// curatedInternalMetrics is the set of internal telemetry registry metric families reported when
// `internal_telemetry.enabled` is set. It is the in-process equivalent of the expvar paths listed
// in cmd/agent/dist/conf.d/go_expvar.d/agent_stats.yaml.example, so that customers monitoring the
// Agent can drop the go_expvar instance and its HTTP scrape entirely.
//
// Names here are Prometheus metric family names from the regular (non-default) registry. They are
// reported as `datadog.agent.<name with __ replaced by .>`, and the Prometheus labels become
// metric tags, so a single family covers several expvar paths that were only distinguishable by
// name (for example `dogstatsd/MetricPackets` and `dogstatsd/MetricParseErrors` are both
// `dogstatsd__processed`, separated by the `state` tag).
//
// One path in the expvar example has no counterpart here, on purpose:
// `scheduler/Queues/<n>/Interval` is redundant with the `interval` tag on
// `scheduler__queue_size`.
var curatedInternalMetrics = []string{
	// Forwarder. `transactions__errors` covers Errors and every ErrorsByType/* path via its
	// `error_type` tag (dns_lookup_failure, tls_handshake_failure, connection_failure,
	// writing_failure, invalid_request).
	"transactions__retry_queue_size",
	"transactions__success",
	"transactions__errors",
	"transactions__http_errors",

	// DogStatsD. The `state` tag separates successful packets from reading errors, and
	// `message_type` separates metrics from events and service checks.
	"dogstatsd__udp_packets",
	"dogstatsd__uds_packets",
	"dogstatsd__uds_origin_detection_error",
	"dogstatsd__processed",

	// Aggregator. `aggregator__flush` covers the *Flushed counters and `aggregator__processed`
	// covers the per-data-type sample counters.
	"aggregator__dogstatsd_contexts",
	"aggregator__flush",
	"aggregator__processed",
	"aggregator__flush_time",
	"aggregator__flush_count",
	"aggregator__number_of_flush",

	// Check scheduler.
	"scheduler__checks_entered",
	"scheduler__queues_count",
	"scheduler__queue_size",

	// Logs agent.
	"logs__running",
	"logs__network_errors",
	"logs__decoded",
	"logs__processed",
	"logs__sent",
	"logs__bytes_sent",
	"logs__encoded_bytes_sent",
	"logs_client_http_destination__idle_ms",
	"logs_client_http_destination__in_use_ms",
}
