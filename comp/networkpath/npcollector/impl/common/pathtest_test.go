// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
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

// TestPathtest_GetHash_Origin verifies that pathtests differing only by Origin
// produce different hashes (they must not be deduplicated together).
func TestPathtest_GetHash_Origin(t *testing.T) {
	base := Pathtest{
		Hostname:          "host1",
		Port:              443,
		Protocol:          "TCP",
		SourceContainerID: "",
	}
	withAgentOrigin := base
	withAgentOrigin.Origin = model.OriginAgentTraffic

	withDeviceOrigin := base
	withDeviceOrigin.Origin = model.OriginNetworkDevice

	assert.NotEqual(t, withAgentOrigin.GetHash(), withDeviceOrigin.GetHash(),
		"pathtests with different Origin values must produce different hashes")
}

// TestPathtest_GetHash_MetadataNotIncluded verifies that pathtests differing only in
// metadata fields (DestIP, Namespaces, ExporterAddrs) produce the same hash so they
// are correctly deduplicated.
func TestPathtest_GetHash_MetadataNotIncluded(t *testing.T) {
	base := Pathtest{
		Hostname:          "host1",
		Port:              443,
		Protocol:          "TCP",
		SourceContainerID: "",
		Origin:            model.OriginNetworkDevice,
	}

	// Differ only by DestIP — hash must be the same.
	withDestIP1 := base
	withDestIP1.DestIP = netip.MustParseAddr("1.2.3.4")

	withDestIP2 := base
	withDestIP2.DestIP = netip.MustParseAddr("5.6.7.8")

	assert.Equal(t, withDestIP1.GetHash(), withDestIP2.GetHash(),
		"pathtests differing only by DestIP must produce the same hash")

	// Differ only by Namespaces — hash must be the same.
	withNS1 := base
	withNS1.Metadata.Namespaces = []string{"ns1"}

	withNS2 := base
	withNS2.Metadata.Namespaces = []string{"ns2"}

	assert.Equal(t, withNS1.GetHash(), withNS2.GetHash(),
		"pathtests differing only by Namespaces must produce the same hash")

	// Differ only by ExporterAddrs — hash must be the same.
	withExp1 := base
	withExp1.Metadata.ExporterAddrs = []netip.Addr{netip.MustParseAddr("10.0.0.1")}

	withExp2 := base
	withExp2.Metadata.ExporterAddrs = []netip.Addr{netip.MustParseAddr("10.0.0.2")}

	assert.Equal(t, withExp1.GetHash(), withExp2.GetHash(),
		"pathtests differing only by ExporterAddrs must produce the same hash")
}
