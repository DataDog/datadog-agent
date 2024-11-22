// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ignore

package protocols

// #cgo CFLAGS: -I ../../ebpf/c  -I ../ebpf/c
// #include "../ebpf/c/protocols/classification/defs.h"
// #include "../ebpf/c/protocols/postgres/types.h"
import "C"

const (
	layerAPIBit         = C.LAYER_API_BIT
	layerApplicationBit = C.LAYER_APPLICATION_BIT
	layerEncryptionBit  = C.LAYER_ENCRYPTION_BIT
)

const (
	// PostgresMaxMessagesPerTailCall is the maximum number of messages that can be processed in a single tail call in our Postgres decoding solution
	PostgresMaxMessagesPerTailCall = C.POSTGRES_MAX_MESSAGES_PER_TAIL_CALL
	// PostgresMaxTailCalls is the maximum number of tail calls that can be made in our Postgres decoding solution
	PostgresMaxTailCalls = C.POSTGRES_MAX_TAIL_CALLS_FOR_MAX_MESSAGES
)

// DispatcherProgramType is a C type to represent the eBPF programs used for tail calls.
type DispatcherProgramType C.dispatcher_prog_t

const (
	// DispatcherKafkaProg is the Golang representation of the C.DISPATCHER_KAFKA_PROG enum.
	DispatcherKafkaProg DispatcherProgramType = C.DISPATCHER_KAFKA_PROG
)

// ProgramType is a C type to represent the eBPF programs used for tail calls.
type ProgramType C.protocol_prog_t

const (
	// ProgramHTTP is the Golang representation of the C.PROG_HTTP enum
	ProgramHTTP ProgramType = C.PROG_HTTP
	// ProgramHTTPTermination is tail call to process http termination.
	ProgramHTTPTermination ProgramType = C.PROG_HTTP_TERMINATION
	// ProgramHTTP2HandleFirstFrame is the Golang representation of the C.PROG_HTTP2_HANDLE_FIRST_FRAME enum
	ProgramHTTP2HandleFirstFrame ProgramType = C.PROG_HTTP2_HANDLE_FIRST_FRAME
	// ProgramHTTP2FrameFilter is the Golang representation of the C.PROG_HTTP2_HANDLE_FRAME enum
	ProgramHTTP2FrameFilter ProgramType = C.PROG_HTTP2_FRAME_FILTER
	// ProgramHTTP2HeadersParser is the Golang representation of the C.PROG_HTTP2_HEADERS_PARSER enum
	ProgramHTTP2HeadersParser ProgramType = C.PROG_HTTP2_HEADERS_PARSER
	// ProgramHTTP2DynamicTableCleaner is the Golang representation of the C.PROG_HTTP2_DYNAMIC_TABLE_CLEANER enum
	ProgramHTTP2DynamicTableCleaner ProgramType = C.PROG_HTTP2_DYNAMIC_TABLE_CLEANER
	// ProgramHTTP2EOSParser is the Golang representation of the C.PROG_HTTP2_EOS_PARSER enum
	ProgramHTTP2EOSParser ProgramType = C.PROG_HTTP2_EOS_PARSER
	// ProgramHTTP2Termination is tail call to process HTTP2 termination.
	ProgramHTTP2Termination ProgramType = C.PROG_HTTP2_TERMINATION
	// ProgramKafka is the Golang representation of the C.PROG_KAFKA enum
	ProgramKafka ProgramType = C.PROG_KAFKA
	// ProgramKafkaFetchResponsePartitionParserV0 is the Golang representation of the C.PROG_KAFKA_FETCH_RESPONSE_PARTITION_PARSER_V0 enum
	ProgramKafkaFetchResponsePartitionParserV0 ProgramType = C.PROG_KAFKA_FETCH_RESPONSE_PARTITION_PARSER_V0
	// ProgramKafkaFetchResponsePartitionParserV12 is the Golang representation of the C.PROG_KAFKA_FETCH_RESPONSE_PARTITION_PARSER_V12 enum
	ProgramKafkaFetchResponsePartitionParserV12 ProgramType = C.PROG_KAFKA_FETCH_RESPONSE_PARTITION_PARSER_V12
	// ProgramKafkaFetchResponseRecordBatchParserV0 is the Golang representation of the C.PROG_KAFKA_FETCH_RESPONSE_RECORD_BATCH_PARSER_V0 enum
	ProgramKafkaFetchResponseRecordBatchParserV0 ProgramType = C.PROG_KAFKA_FETCH_RESPONSE_RECORD_BATCH_PARSER_V0
	// ProgramKafkaFetchResponseRecordBatchParserV12 is the Golang representation of the C.PROG_KAFKA_FETCH_RESPONSE_RECORD_BATCH_PARSER_V12 enum
	ProgramKafkaFetchResponseRecordBatchParserV12 ProgramType = C.PROG_KAFKA_FETCH_RESPONSE_RECORD_BATCH_PARSER_V12
	// ProgramKafkaProduceResponsePartitionParserV0 is the Golang representation of the C.PROG_KAFKA_PRODUCE_RESPONSE_PARTITION_PARSER_V0 enum
	ProgramKafkaProduceResponsePartitionParserV0 ProgramType = C.PROG_KAFKA_PRODUCE_RESPONSE_PARTITION_PARSER_V0
	// ProgramKafkaProduceResponsePartitionParserV9 is the Golang representation of the C.PROG_KAFKA_PRODUCE_RESPONSE_PARTITION_PARSER_V9 enum
	ProgramKafkaProduceResponsePartitionParserV9 ProgramType = C.PROG_KAFKA_PRODUCE_RESPONSE_PARTITION_PARSER_V9
	// ProgramKafkaTermination is tail call to process Kafka termination.
	ProgramKafkaTermination ProgramType = C.PROG_KAFKA_TERMINATION
	// ProgramPostgres is the Golang representation of the C.PROG_POSTGRES enum
	ProgramPostgres ProgramType = C.PROG_POSTGRES
	// ProgramPostgresHandleResponse is the Golang representation of the C.PROG_POSTGRES_HANDLE_RESPONSE enum
	ProgramPostgresHandleResponse ProgramType = C.PROG_POSTGRES_HANDLE_RESPONSE
	// ProgramPostgresParseMessage is the Golang representation of the C.PROG_POSTGRES_PROCESS_PARSE_MESSAGE enum
	ProgramPostgresParseMessage ProgramType = C.PROG_POSTGRES_PROCESS_PARSE_MESSAGE
	// ProgramPostgresTermination is tail call to process Postgres termination.
	ProgramPostgresTermination ProgramType = C.PROG_POSTGRES_TERMINATION
	// ProgramRedis is the Golang representation of the C.PROG_REDIS enum
	ProgramRedis ProgramType = C.PROG_REDIS
	// ProgramRedisTermination is the Golang representation of the C.PROG_REDIS_TERMINATION enum
	ProgramRedisTermination ProgramType = C.PROG_REDIS_TERMINATION
)

type ebpfProtocolType C.protocol_t

const (
	// Unknown is the default value, protocol was not detected
	ebpfUnknown ebpfProtocolType = C.PROTOCOL_UNKNOWN
	// HTTP protocol
	ebpfHTTP ebpfProtocolType = C.PROTOCOL_HTTP
	// HTTP2 protocol
	ebpfHTTP2 ebpfProtocolType = C.PROTOCOL_HTTP2
	// Kafka protocol
	ebpfKafka ebpfProtocolType = C.PROTOCOL_KAFKA
	// TLS protocol
	ebpfTLS ebpfProtocolType = C.PROTOCOL_TLS
	// Mongo protocol
	ebpfMongo ebpfProtocolType = C.PROTOCOL_MONGO
	// Postgres protocol
	ebpfPostgres ebpfProtocolType = C.PROTOCOL_POSTGRES
	// AMQP protocol
	ebpfAMQP ebpfProtocolType = C.PROTOCOL_AMQP
	// Redis protocol
	ebpfRedis ebpfProtocolType = C.PROTOCOL_REDIS
	// MySQL protocol
	ebpfMySQL ebpfProtocolType = C.PROTOCOL_MYSQL
	// GRPC protocol
	ebpfGRPC ebpfProtocolType = C.PROTOCOL_GRPC
)
