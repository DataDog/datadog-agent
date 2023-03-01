// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network"
)

func TestFormatProtocols(t *testing.T) {
	tests := []struct {
		name       string
		protocol   network.ProtocolType
		staticTags uint64
		want       *model.ProtocolStack
	}{
		{
			name:     "unknown protocol",
			protocol: network.ProtocolUnknown,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolUnknown,
				},
			},
		},
		{
			name:     "unclassified protocol",
			protocol: network.ProtocolUnclassified,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolUnknown,
				},
			},
		},
		{
			name:     "http protocol",
			protocol: network.ProtocolHTTP,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolHTTP,
				},
			},
		},
		{
			name:     "kafka protocol",
			protocol: network.ProtocolKafka,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolKafka,
				},
			},
		},
		{
			name:     "amqp protocol",
			protocol: network.ProtocolAMQP,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolAMQP,
				},
			},
		},
		{
			name:     "redis protocol",
			protocol: network.ProtocolRedis,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolRedis,
				},
			},
		},
		{
			name:     "mongo protocol",
			protocol: network.ProtocolMongo,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolMongo,
				},
			},
		},
		{
			name:     "mysql protocol",
			protocol: network.ProtocolMySQL,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolMySQL,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, formatProtocol(tt.protocol, tt.staticTags), "formatProtocol(%v)", tt.protocol)
		})
	}
}
