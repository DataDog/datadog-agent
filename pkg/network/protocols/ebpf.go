// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package protocols

import "github.com/DataDog/datadog-agent/pkg/util/log"

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

	protocol := uint16(protoNum) | layerBit
	switch ebpfProtocolType(protocol) {
	case ebpfUnknown:
		return Unknown
	case ebpfGRPC:
		return GRPC
	case ebpfHTTP:
		return HTTP
	case ebpfHTTP2:
		return HTTP2
	case ebpfKafka:
		return Kafka
	case ebpfTLS:
		return TLS
	case ebpfMongo:
		return Mongo
	case ebpfPostgres:
		return Postgres
	case ebpfAMQP:
		return AMQP
	case ebpfRedis:
		return Redis
	case ebpfMySQL:
		return MySQL
	default:
		log.Errorf("unknown eBPF protocol type: %x", protocol)
		return Unknown
	}
}
