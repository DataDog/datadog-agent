// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	model "github.com/DataDog/agent-payload/v5/process"
)

type ProtocolType uint16

const (
	ProtocolUnclassified = ProtocolType(model.ProtocolType_protocolUnclassified)
	ProtocolUnknown      = ProtocolType(model.ProtocolType_protocolUnknown)
	ProtocolHTTP         = ProtocolType(model.ProtocolType_protocolHTTP)
	ProtocolHTTP2        = ProtocolType(model.ProtocolType_protocolHTTP2)
	ProtocolTLS          = ProtocolType(model.ProtocolType_protocolTLS)
	ProtocolKafka        = ProtocolType(model.ProtocolType_protocolKafka)
	ProtocolMongo        = ProtocolType(model.ProtocolType_protocolMongo)
	ProtocolPostgres     = ProtocolType(model.ProtocolType_protocolPostgres)
	ProtocolAMQP         = ProtocolType(model.ProtocolType_protocolAMQP)
	ProtocolRedis        = ProtocolType(model.ProtocolType_protocolRedis)
	ProtocolMySQL        = ProtocolType(model.ProtocolType_protocolMySQL)
)

var (
	supportedProtocols = map[ProtocolType]struct{}{
		ProtocolUnclassified: {},
		ProtocolUnknown:      {},
		ProtocolHTTP:         {},
		ProtocolHTTP2:        {},
		ProtocolTLS:          {},
		ProtocolKafka:        {},
		ProtocolMongo:        {},
		ProtocolPostgres:     {},
		ProtocolAMQP:         {},
		ProtocolRedis:        {},
		ProtocolMySQL:        {},
	}
)

// IsValidProtocolValue checks if a given value is a valid protocol.
func IsValidProtocolValue(val uint8) bool {
	_, ok := supportedProtocols[ProtocolType(val)]
	return ok
}
