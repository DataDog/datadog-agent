// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package protocols

// #cgo CFLAGS: -I ../../ebpf/c  -I ../ebpf/c
// #include "../ebpf/c/protocols/classification/defs.h"
import "C"

import "github.com/DataDog/datadog-agent/pkg/util/log"

const (
	layerAPIBit         = C.LAYER_API_BIT
	layerApplicationBit = C.LAYER_APPLICATION_BIT
	layerEncryptionBit  = C.LAYER_ENCRYPTION_BIT
)

// DispatcherProgramType is a C type to represent the eBPF programs used for tail calls.
type DispatcherProgramType C.dispatcher_prog_t

const (
	// DispatcherKafkaProg is the Golang representation of the C.DISPATCHER_KAFKA_PROG enum.
	DispatcherKafkaProg DispatcherProgramType = C.DISPATCHER_KAFKA_PROG
)

// TLSDispatcherProgramType is a C type to represent the eBPF programs used for TLS tail calls.
type TLSDispatcherProgramType C.tls_dispatcher_prog_t

const (
	// TLSDispatcherKafkaProg is the Golang representation of the C.TLS_DISPATCHER_KAFKA_PROG enum.
	TLSDispatcherKafkaProg TLSDispatcherProgramType = C.TLS_DISPATCHER_KAFKA_PROG
)

// ProgramType is a C type to represent the eBPF programs used for tail calls.
type ProgramType C.protocol_prog_t

const (
	// ProgramHTTP is the Golang representation of the C.PROG_HTTP enum
	ProgramHTTP ProgramType = C.PROG_HTTP
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
	// ProgramKafka is the Golang representation of the C.PROG_KAFKA enum
	ProgramKafka ProgramType = C.PROG_KAFKA
	// ProgramKafkaResponseParser is the Golang representation of the C.PROG_KAFKA_RESPONSE_PARSER enum
	ProgramKafkaResponseParser ProgramType = C.PROG_KAFKA_RESPONSE_PARSER
	// ProgramPostgres is the Golang representation of the C.PROG_POSTGRES enum
	ProgramPostgres ProgramType = C.PROG_POSTGRES
	// ProgramPostgresParseMessage is the Golang representation of the C.PROG_POSTGRES_PROCESS_PARSE_MESSAGE enum
	ProgramPostgresParseMessage ProgramType = C.PROG_POSTGRES_PROCESS_PARSE_MESSAGE
)

// Application layer of the protocol stack.
func Application(protoNum uint8) ProtocolType {
	return toProtocolType(protoNum, layerApplicationBit)
}

// API layer of the protocol stack.
func API(protoNum uint8) ProtocolType {
	return toProtocolType(protoNum, layerAPIBit)
}

// Encryption layer of the protocol stack.
func Encryption(protoNum uint8) ProtocolType {
	return toProtocolType(protoNum, layerEncryptionBit)
}

func toProtocolType(protoNum uint8, layerBit uint16) ProtocolType {
	if protoNum == 0 {
		return Unknown
	}

	protocol := uint16(protoNum) | uint16(layerBit)
	switch protocol {
	case C.PROTOCOL_UNKNOWN:
		return Unknown
	case C.PROTOCOL_GRPC:
		return GRPC
	case C.PROTOCOL_HTTP:
		return HTTP
	case C.PROTOCOL_HTTP2:
		return HTTP2
	case C.PROTOCOL_KAFKA:
		return Kafka
	case C.PROTOCOL_TLS:
		return TLS
	case C.PROTOCOL_MONGO:
		return Mongo
	case C.PROTOCOL_POSTGRES:
		return Postgres
	case C.PROTOCOL_AMQP:
		return AMQP
	case C.PROTOCOL_REDIS:
		return Redis
	case C.PROTOCOL_MYSQL:
		return MySQL
	default:
		log.Errorf("unknown eBPF protocol type: %x", protocol)
		return Unknown
	}
}

// TLSProgramType is a C type to represent the eBPF programs used for tail calls
// in TLS traffic decoding
type TLSProgramType C.tls_prog_t

const (
	// ProgramTLSHTTPProcess is tail call to process http traffic.
	ProgramTLSHTTPProcess TLSProgramType = C.TLS_HTTP_PROCESS
	// ProgramTLSHTTPTermination is tail call to process http termination.
	ProgramTLSHTTPTermination TLSProgramType = C.TLS_HTTP_TERMINATION
	// ProgramTLSHTTP2FirstFrame is tail call and the entry point of the TLS HTTP2 decoding.
	ProgramTLSHTTP2FirstFrame TLSProgramType = C.TLS_HTTP2_FIRST_FRAME
	// ProgramTLSHTTP2Filter is tail call to filter http2 frames.
	ProgramTLSHTTP2Filter TLSProgramType = C.TLS_HTTP2_FILTER
	// ProgramTLSHTTP2HeaderParser is tail call to parse the previously filtered http2 header frames.
	ProgramTLSHTTP2HeaderParser TLSProgramType = C.TLS_HTTP2_HEADERS_PARSER
	// ProgramTLSHTTP2DynamicTableCleaner is tail call to clean the dynamic table.
	ProgramTLSHTTP2DynamicTableCleaner TLSProgramType = C.TLS_HTTP2_DYNAMIC_TABLE_CLEANER
	// ProgramTLSHTTP2EOSParser is tail call to process End-Of-Stream frames.
	ProgramTLSHTTP2EOSParser TLSProgramType = C.TLS_HTTP2_EOS_PARSER
	// ProgramTLSHTTP2Termination is tail call to process TLS HTTP2 termination.
	ProgramTLSHTTP2Termination TLSProgramType = C.TLS_HTTP2_TERMINATION
	// ProgramTLSKafka is tail call to process Kafka TLS frames,
	ProgramTLSKafka TLSProgramType = C.TLS_KAFKA
	// ProgramTLSKafkaResponseParser is tail call to process Kafka TLS responses,
	ProgramTLSKafkaResponseParser TLSProgramType = C.TLS_KAFKA_RESPONSE_PARSER
	// ProgramTLSKafkaTermination is tail call to process Kafka TLS termination.
	ProgramTLSKafkaTermination TLSProgramType = C.TLS_KAFKA_TERMINATION
	// ProgramTLSPostgres is tail call to process Postgres TLS frames.
	ProgramTLSPostgres TLSProgramType = C.TLS_POSTGRES
	// ProgramTLSPostgresParseMessage is tail call to process Parse message of Postgres frames.
	ProgramTLSPostgresParseMessage TLSProgramType = C.TLS_PROG_POSTGRES_PROCESS_PARSE_MESSAGE
	// ProgramTLSPostgresTermination is tail call to process Postgres TLS termination.
	ProgramTLSPostgresTermination TLSProgramType = C.TLS_POSTGRES_TERMINATION
)
