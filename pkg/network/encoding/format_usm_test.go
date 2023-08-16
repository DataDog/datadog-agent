// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
)

func TestFormatProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol protocols.ProtocolType
		want     *model.ProtocolStack
	}{
		{
			name:     "unknown protocol",
			protocol: protocols.ProtocolUnknown,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolUnknown,
				},
			},
		},
		{
			name:     "unclassified protocol",
			protocol: protocols.ProtocolUnclassified,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolUnknown,
				},
			},
		},
		{
			name:     "http protocol",
			protocol: protocols.ProtocolHTTP,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolHTTP,
				},
			},
		},
		{
			name:     "http2 protocol",
			protocol: protocols.ProtocolHTTP2,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolHTTP2,
				},
			},
		},
		{
			name:     "tls protocol",
			protocol: protocols.ProtocolTLS,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolTLS,
				},
			},
		},
		{
			name:     "kafka protocol",
			protocol: protocols.ProtocolKafka,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolKafka,
				},
			},
		},
		{
			name:     "amqp protocol",
			protocol: protocols.ProtocolAMQP,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolAMQP,
				},
			},
		},
		{
			name:     "redis protocol",
			protocol: protocols.ProtocolRedis,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolRedis,
				},
			},
		},
		{
			name:     "mongo protocol",
			protocol: protocols.ProtocolMongo,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolMongo,
				},
			},
		},
		{
			name:     "mysql protocol",
			protocol: protocols.ProtocolMySQL,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolMySQL,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, formatProtocol(tt.protocol, 0), "formatProtocol(%v)", tt.protocol)
		})
	}
}
