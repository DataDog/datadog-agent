// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// formatProtocolStack generates a protobuf representation of protocol stack  from the internal one (`protocols.Stack`)
// i.e: if the input is protocols.Stack{Application: protocols.HTTP2} the output should be:
//
//	&model.ProtocolStack{
//			Stack: []model.ProtocolType{
//				model.ProtocolType_protocolHTTP2,
//			},
//		}
//
// Additionnally, if the staticTags contains TLS tags, the TLS protocol is added
// to the protocol stack, giving an output like this:
//
//	&model.ProtocolStack{
//			Stack: []model.ProtocolType{
//				model.ProtocolType_protocolTLS,
//				model.ProtocolType_protocolHTTP2,
//			},
//		}
func formatProtocolStack(originalStack protocols.Stack, staticTags uint64) *model.ProtocolStack {
	stack := make([]model.ProtocolType, 0, 1)
	if network.IsTLSTag(staticTags) {
		stack = append(stack, formatProtocol(protocols.TLS))
	}

	stack = append(stack, formatProtocol(originalStack.Application))

	// TODO: decide how we want to encode the case where multiple layers are unknown
	// stack = append(stack, formatProtocol(originalStack.Api))
	// stack = append(stack, formatProtocol(originalStack.Encryption))

	return &model.ProtocolStack{
		Stack: stack,
	}
}

func formatProtocol(proto protocols.ProtocolType) model.ProtocolType {
	switch proto {
	case protocols.Unknown:
		return model.ProtocolType_protocolUnknown
	case protocols.HTTP:
		return model.ProtocolType_protocolHTTP
	case protocols.HTTP2:
		return model.ProtocolType_protocolHTTP2
	case protocols.TLS:
		return model.ProtocolType_protocolTLS
	case protocols.Kafka:
		return model.ProtocolType_protocolKafka
	case protocols.Mongo:
		return model.ProtocolType_protocolMongo
	case protocols.Postgres:
		return model.ProtocolType_protocolPostgres
	case protocols.AMQP:
		return model.ProtocolType_protocolAMQP
	case protocols.Redis:
		return model.ProtocolType_protocolRedis
	case protocols.MySQL:
		return model.ProtocolType_protocolMySQL
	default:
		log.Warnf("missing protobuf representation for protocol %d", proto)
		return model.ProtocolType_protocolUnknown
	}
}
