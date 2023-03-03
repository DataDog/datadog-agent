// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package protocols

/*
#include "../ebpf/c/protocols/classification/defs.h"
*/
import "C"

type ProtocolType = C.protocol_t

const (
	Unknown  ProtocolType = C.PROTOCOL_UNKNOWN
	HTTP     ProtocolType = C.PROTOCOL_HTTP
	HTTP2    ProtocolType = C.PROTOCOL_HTTP2
	Kafka    ProtocolType = C.PROTOCOL_KAFKA
	TLS      ProtocolType = C.PROTOCOL_TLS
	Mongo    ProtocolType = C.PROTOCOL_MONGO
	Postgres ProtocolType = C.PROTOCOL_POSTGRES
	AMQP     ProtocolType = C.PROTOCOL_AMQP
	Redis    ProtocolType = C.PROTOCOL_REDIS
	MySQL    ProtocolType = C.PROTOCOL_MYSQL
)

type DispatcherProgramType C.dispatcher_prog_t

const (
	DispatcherKafkaProg DispatcherProgramType = C.DISPATCHER_KAFKA_PROG
)

type ProgramType C.protocol_prog_t

const (
	ProgramHTTP  ProgramType = C.PROG_HTTP
	ProgramHTTP2 ProgramType = C.PROG_HTTP2
	ProgramKafka ProgramType = C.PROG_KAFKA
)

const (
	layerAPIBit         = C.LAYER_API_BIT
	layerApplicationBit = C.LAYER_APPLICATION_BIT
	layerEncryptionBit  = C.LAYER_ENCRYPTION_BIT
)
