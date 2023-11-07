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

type DispatcherProgramType C.dispatcher_prog_t

const (
	DispatcherKafkaProg DispatcherProgramType = C.DISPATCHER_KAFKA_PROG
)

type ProgramType C.protocol_prog_t

const (
	// ProgramHTTP is the Golang representation of the C.PROG_HTTP enum
	ProgramHTTP ProgramType = C.PROG_HTTP
	// ProgramHTTP2HandleFirstFrame is the Golang representation of the C.PROG_HTTP2_HANDLE_FIRST_FRAME enum
	ProgramHTTP2HandleFirstFrame ProgramType = C.PROG_HTTP2_HANDLE_FIRST_FRAME
	// ProgramHTTP2FrameFilter is the Golang representation of the C.PROG_HTTP2_HANDLE_FRAME enum
	ProgramHTTP2FrameFilter ProgramType = C.PROG_HTTP2_FRAME_FILTER
	// ProgramHTTP2FrameParser is the Golang representation of the C.PROG_HTTP2_FRAME_PARSER enum
	ProgramHTTP2FrameParser ProgramType = C.PROG_HTTP2_FRAME_PARSER
	// ProgramKafka is the Golang representation of the C.PROG_KAFKA enum
	ProgramKafka ProgramType = C.PROG_KAFKA
)

func Application(protoNum uint8) ProtocolType {
	return toProtocolType(protoNum, layerApplicationBit)
}

func API(protoNum uint8) ProtocolType {
	return toProtocolType(protoNum, layerAPIBit)
}

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
	// ProgramTLSHTTP2 is tail call to process http2 traffic.
	ProgramTLSHTTP2 TLSProgramType = C.TLS_HTTP2_PROCESS
	// ProgramTLSHTTP2FramesParserFromState is tail call to process http2 frames when state is present.
	ProgramTLSHTTP2FramesParserFromState TLSProgramType = C.TLS_HTTP2_FRAMES_PARSER_FROM_STATE
	// ProgramTLSHTTP2FramesParserNoState is tail call to process http2 frames fully contained in one buffer.
	ProgramTLSHTTP2FramesParserNoState TLSProgramType = C.TLS_HTTP2_FRAMES_PARSER_NO_STATE
	// ProgramTLSHTTP2Termination is tail call to process http2 termination..
	ProgramTLSHTTP2Termination TLSProgramType = C.TLS_HTTP2_TERMINATION
)
