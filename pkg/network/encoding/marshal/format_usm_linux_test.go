// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package marshal

import (
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

func TestFormatTLSProtocols(t *testing.T) {
	tests := []struct {
		name       string
		protocol   protocols.Stack
		staticTags uint64
		want       *model.ProtocolStack
	}{
		{
			name:       "GnuTLS - unknown protocol",
			protocol:   protocols.Stack{Application: protocols.Unknown},
			staticTags: http.TLS | http.GnuTLS,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolTLS,
				},
			},
		},
		{
			name:       "OpenSSL - HTTP protocol",
			protocol:   protocols.Stack{Application: protocols.HTTP},
			staticTags: http.TLS | http.OpenSSL,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolTLS,
					model.ProtocolType_protocolHTTP,
				},
			},
		},
		{
			name:       "GoTLS - MySQL protocol",
			protocol:   protocols.Stack{Application: protocols.MySQL},
			staticTags: http.TLS | http.Go,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolTLS,
					model.ProtocolType_protocolMySQL,
				},
			},
		},
		{
			name:       "Unknown static tags - MySQL protocol",
			protocol:   protocols.Stack{Application: protocols.MySQL},
			staticTags: 0x80000000,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolMySQL,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, FormatProtocolStack(tt.protocol, tt.staticTags), "formatProtocol(%v)", tt.protocol)
		})
	}
}
