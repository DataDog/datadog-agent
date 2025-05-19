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
