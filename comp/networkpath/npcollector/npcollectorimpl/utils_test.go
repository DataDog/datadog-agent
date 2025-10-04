// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/stretchr/testify/assert"
)

func Test_convertProtocol(t *testing.T) {
	assert.Equal(t, convertProtocol(model.ConnectionType_udp), payload.ProtocolUDP)
	assert.Equal(t, convertProtocol(model.ConnectionType_tcp), payload.ProtocolTCP)
}

func Test_shouldSkipDomain(t *testing.T) {
	tests := []struct {
		name       string
		domain     string
		ddSite     string
		shouldSkip bool
	}{
		{
			name:       "not skip",
			domain:     "example.com",
			shouldSkip: false,
		},
		{
			name:       "skip .ec2.internal",
			domain:     "ip-10-113-0-225.ec2.internal",
			shouldSkip: true,
		},
		{
			name:       "skip datadog site",
			domain:     "intake.datadoghq.com",
			ddSite:     "datadoghq.com",
			shouldSkip: true,
		},
		{
			name:       "skip elb format1",
			domain:     "lb-web-public-shard0-1546910068.us-east-1.elb.amazonaws.com",
			shouldSkip: true,
		},
		{
			name:       "skip elb format2",
			domain:     "l4-web-public-s1-7109eb0b808a5bbd.elb.us-east-1.amazonaws.com",
			shouldSkip: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.shouldSkip, shouldSkipDomain(tt.domain, tt.ddSite), "shouldSkipDomain(%v, %v)", tt.domain, tt.ddSite)
		})
	}
}
