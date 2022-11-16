// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package kafka

/*
#include "../../ebpf/c/tracer.h"
#include "../../ebpf/c/kafka/kafka-types.h"
*/
import "C"

type kafkaConnTuple C.conn_tuple_t
type kafkaBatchState C.kafka_batch_state_t

//type sslSock C.ssl_sock_t
//type sslReadArgs C.ssl_read_args_t
//
type ebpfKafkaTx C.kafka_transaction_t
type kafkaBatch C.kafka_batch_t
type kafkaBatchKey C.kafka_batch_key_t

//
//type libPath C.lib_path_t
//
const (
	KAFKABatchSize  = C.KAFKA_BATCH_SIZE
	KAFKABatchPages = C.KAFKA_BATCH_PAGES
	KAFKABufferSize = C.KAFKA_BUFFER_SIZE

	kafkaProg = C.KAFKA_PROG

	libPathMaxSize = C.LIB_PATH_MAX_SIZE
)
