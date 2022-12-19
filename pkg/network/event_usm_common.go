// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

type ProtocolType uint16

const (
	ProtocolUnclassified ProtocolType = iota
	ProtocolUnknown
	ProtocolHTTP
	ProtocolHTTP2
	ProtocolTLS
	ProtocolMongo = 6
	ProtocolAMQP  = 8
	ProtocolRedis = 9
	MaxProtocols  = 10
)
