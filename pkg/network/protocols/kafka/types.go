// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package kafka

/*
#include "../../ebpf/c/conn_tuple.h"
#include "../../ebpf/c/protocols/kafka/types.h"
*/
import "C"

const (
	TopicNameBuckets = C.KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS
)

type ConnTuple C.conn_tuple_t

type EbpfTx C.kafka_transaction_t

type RawKernelTelemetry C.kafka_telemetry_t
