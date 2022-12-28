// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"github.com/stretchr/testify/assert"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network"
)

func TestFormatProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol network.ProtocolType
		want     *model.ProtocolStack
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
			name:     "amqp protocol",
			protocol: network.ProtocolAMQP,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolAMQP,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, formatProtocol(tt.protocol), "formatProtocol(%v)", tt.protocol)
		})
	}
}
