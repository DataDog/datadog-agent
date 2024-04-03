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

type ConnTuple C.conn_tuple_t

type EbpfTx C.kafka_transaction_t

type KafkaTransactionKey C.kafka_transaction_key_t
type KafkaTransaction C.kafka_transaction_t

type KafkaResponseContext C.kafka_response_context_t
