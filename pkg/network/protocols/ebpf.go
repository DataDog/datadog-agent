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

// FromProtocolType converts a ProtocolType to its corresponding protocol number.
// It returns 0 if the ProtocolType is not supported.
func FromProtocolType(protocolType ProtocolType) uint8 {
	switch protocolType {
	case HTTP:
		return uint8((uint16(ebpfHTTP) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case HTTP2:
		return uint8((uint16(ebpfHTTP2) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case Kafka:
		return uint8((uint16(ebpfKafka) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case TLS:
		return uint8((uint16(ebpfTLS) ^ uint16(layerEncryptionBit)) & uint16(0xff))
	case Mongo:
		return uint8((uint16(ebpfMongo) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case Postgres:
		return uint8((uint16(ebpfPostgres) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case AMQP:
		return uint8((uint16(ebpfAMQP) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case Redis:
		return uint8((uint16(ebpfRedis) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case MySQL:
		return uint8((uint16(ebpfMySQL) ^ uint16(layerApplicationBit)) & uint16(0xff))
	case GRPC:
		return uint8((uint16(ebpfGRPC) ^ uint16(layerAPIBit)) & uint16(0xff))
	default:
		return 0
	}
}
