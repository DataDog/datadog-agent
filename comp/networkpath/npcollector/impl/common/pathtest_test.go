// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/stretchr/testify/assert"
)

func TestPathtest_GetHash(t *testing.T) {
	p1 := Pathtest{
		Hostname:          "aaa1",
		Port:              80,
		Protocol:          "TCP",
		SourceContainerID: "containerID1",
	}
	p2 := Pathtest{
		Hostname:          "aaa2",
		Port:              80,
		Protocol:          "TCP",
		SourceContainerID: "containerID1",
	}
	p3 := Pathtest{
		Hostname:          "aaa1",
		Port:              81,
		Protocol:          "TCP",
		SourceContainerID: "containerID1",
	}
	p4 := Pathtest{
		Hostname:          "aaa1",
		Port:              80,
		Protocol:          "UDP",
		SourceContainerID: "containerID1",
	}
	p5 := Pathtest{
		Hostname:          "aaa1",
		Port:              80,
		Protocol:          "TCP",
		SourceContainerID: "containerID2",
	}

	assert.NotEqual(t, p1.GetHash(), p2.GetHash())
	assert.NotEqual(t, p1.GetHash(), p3.GetHash())
	assert.NotEqual(t, p2.GetHash(), p3.GetHash())
	assert.NotEqual(t, p1.GetHash(), p4.GetHash())
	assert.NotEqual(t, p1.GetHash(), p5.GetHash())
}

func TestPathtest_GetHash_Netflow(t *testing.T) {
	udp53 := Pathtest{
		Hostname:  "10.0.0.1",
		Port:      53,
		Protocol:  payload.ProtocolUDP,
		Namespace: "ns1",
		Origin:    payload.PathOriginNetflow,
	}
	udp1234 := Pathtest{
		Hostname:  "10.0.0.1",
		Port:      1234,
		Protocol:  payload.ProtocolUDP,
		Namespace: "ns1",
		Origin:    payload.PathOriginNetflow,
	}
	tcp53 := Pathtest{
		Hostname:  "10.0.0.1",
		Port:      53,
		Protocol:  payload.ProtocolTCP,
		Namespace: "ns1",
		Origin:    payload.PathOriginNetflow,
	}
	tcp1234 := Pathtest{
		Hostname:  "10.0.0.1",
		Port:      1234,
		Protocol:  payload.ProtocolTCP,
		Namespace: "ns1",
		Origin:    payload.PathOriginNetflow,
	}
	otherNamespace := Pathtest{
		Hostname:  "10.0.0.1",
		Port:      53,
		Protocol:  payload.ProtocolUDP,
		Namespace: "ns2",
		Origin:    payload.PathOriginNetflow,
	}
	networkTraffic := Pathtest{
		Hostname: "10.0.0.1",
		Port:     53,
		Protocol: payload.ProtocolUDP,
	}

	assert.Equal(t, udp53.GetHash(), udp1234.GetHash())
	assert.NotEqual(t, tcp53.GetHash(), tcp1234.GetHash())
	assert.NotEqual(t, udp53.GetHash(), otherNamespace.GetHash())
	assert.NotEqual(t, udp53.GetHash(), networkTraffic.GetHash())
}
