// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package kafka

/*
#include "../../ebpf/c/tracer.h"
#include "../../ebpf/c/protocols/kafka/types.h"
*/
import "C"

type kafkaConnTuple C.conn_tuple_t

type EbpfKafkaTx C.kafka_transaction_batch_entry_t
