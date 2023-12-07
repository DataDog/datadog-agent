// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
)

func TestFormatProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol protocols.Stack
		want     *model.ProtocolStack
	}{
		{
			name: "unknown protocol",
			protocol: protocols.Stack{
				Application: protocols.Unknown,
				API:         protocols.Unknown,
				Encryption:  protocols.Unknown,
			},
			want: &model.ProtocolStack{
				Stack: nil,
			},
		},
		{
			name:     "http protocol",
			protocol: protocols.Stack{Application: protocols.HTTP},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolHTTP,
				},
			},
		},
		{
			name:     "http2 protocol",
			protocol: protocols.Stack{Application: protocols.HTTP2},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolHTTP2,
				},
			},
		},
		{
			name:     "grpc protocol",
			protocol: protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolHTTP2,
					model.ProtocolType_protocolGRPC,
				},
			},
		},
		{
			name:     "tls protocol",
			protocol: protocols.Stack{Encryption: protocols.TLS},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolTLS,
				},
			},
		},
		{
			name:     "kafka protocol",
			protocol: protocols.Stack{Application: protocols.Kafka},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolKafka,
				},
			},
		},
		{
			name:     "amqp protocol",
			protocol: protocols.Stack{Application: protocols.AMQP},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolAMQP,
				},
			},
		},
		{
			name:     "redis protocol",
			protocol: protocols.Stack{Application: protocols.Redis},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolRedis,
				},
			},
		},
		{
			name:     "mongo protocol",
			protocol: protocols.Stack{Application: protocols.Mongo},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolMongo,
				},
			},
		},
		{
			name:     "mysql protocol",
			protocol: protocols.Stack{Application: protocols.MySQL},
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolMySQL,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, FormatProtocolStack(tt.protocol, 0), "formatProtocol(%v)", tt.protocol)
		})
	}
}
