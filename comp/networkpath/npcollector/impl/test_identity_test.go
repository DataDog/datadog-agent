// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func TestMakeTestIdentityRFCVector(t *testing.T) {
	got := makeTestIdentity("ip-10-0-0-12.ec2.internal", common.Pathtest{
		Hostname: "api.internal.example.com",
		Port:     443,
		Protocol: payload.ProtocolTCP,
	})

	assert.Equal(t, "88c8b3adcfca20e25d9d640efb51cacf", got)
}

func TestMakeTestIdentityUsesScheduledPathtestFields(t *testing.T) {
	got := makeTestIdentity("agent-hostname", common.Pathtest{
		Hostname: "api.example.com",
		Port:     0,
		Protocol: payload.ProtocolICMP,
	})

	assert.Equal(t, "05d7ac899163f6f3d9d3048d580edf19", got)
}

func TestMakeTestIdentityLengthPrefixesVariableFields(t *testing.T) {
	left := makeTestIdentity("ab", common.Pathtest{
		Hostname: "c",
		Port:     443,
		Protocol: payload.ProtocolTCP,
	})
	right := makeTestIdentity("a", common.Pathtest{
		Hostname: "bc",
		Port:     443,
		Protocol: payload.ProtocolTCP,
	})

	assert.NotEqual(t, left, right)
}

func TestMakeTestIdentityEmptySourceHostname(t *testing.T) {
	got := makeTestIdentity("", common.Pathtest{
		Hostname: "api.internal.example.com",
		Port:     443,
		Protocol: payload.ProtocolTCP,
	})

	assert.Empty(t, got)
}
