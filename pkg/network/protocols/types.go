// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package protocols

type ProtocolType uint16

const (
	Unknown ProtocolType = iota
	HTTP
	HTTP2
	Kafka
	TLS
	Mongo
	Postgres
	AMQP
	Redis
	MySQL
)

func (p ProtocolType) String() string {
	switch p {
	case Unknown:
		return "Unknown"
	case HTTP:
		return "HTTP"
	case HTTP2:
		return "HTTP2"
	case Kafka:
		return "Kafka"
	case TLS:
		return "TLS"
	case Mongo:
		return "Mongo"
	case Postgres:
		return "Postgres"
	case AMQP:
		return "AMPQ"
	case Redis:
		return "Redis"
	case MySQL:
		return "MySQL"
	default:
		// shouldn't happen
		return "Invalid"
	}
}

type Stack struct {
	Api         ProtocolType
	Application ProtocolType
	Encryption  ProtocolType
}

func (s *Stack) MergeWith(other Stack) {
	if s.Api == Unknown {
		s.Api = other.Api
	}

	if s.Application == Unknown {
		s.Application = other.Application
	}

	if s.Encryption == Unknown {
		s.Encryption = other.Encryption
	}
}

func (s *Stack) Contains(proto ProtocolType) bool {
	return s.Api == proto || s.Application == proto || s.Encryption == proto
}
