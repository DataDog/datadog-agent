// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package protocols

// ProtocolType is an enum of supported protocols
type ProtocolType uint8

const (
	// Unknown is the default value, protocol was not detected
	Unknown ProtocolType = iota
	// HTTP protocol
	HTTP
	// HTTP2 protocol
	HTTP2
	// Kafka protocol
	Kafka
	// TLS protocol
	TLS
	// Mongo protocol
	Mongo
	// Postgres protocol
	Postgres
	// AMQP protocol
	AMQP
	// Redis protocol
	Redis
	// MySQL protocol
	MySQL
	// GRPC protocol
	GRPC
)

// String returns the string representation of the protocol
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
		return "AMQP"
	case Redis:
		return "Redis"
	case MySQL:
		return "MySQL"
	case GRPC:
		return "gRPC"
	default:
		// shouldn't happen
		return "Invalid"
	}
}

// Stack is a set of protocols detected on a connection
type Stack struct {
	API         ProtocolType
	Application ProtocolType
	Encryption  ProtocolType
}

// MergeWith merges the other stack into the current one
func (s *Stack) MergeWith(other Stack) {
	if s.API == Unknown {
		s.API = other.API
	}

	if s.Application == Unknown {
		s.Application = other.Application
	}

	if s.Encryption == Unknown {
		s.Encryption = other.Encryption
	}
}

// Contains returns true if the stack contains the given protocol
func (s *Stack) Contains(proto ProtocolType) bool {
	return s.API == proto || s.Application == proto || s.Encryption == proto
}

// IsUnknown returns true if all protocol types are `Unknown`
func (s *Stack) IsUnknown() bool {
	return s.API == Unknown && s.Application == Unknown && s.Encryption == Unknown
}
