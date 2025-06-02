// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/slice"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestDNATIntraHost(t *testing.T) {
	ns := networkState{}
	DNatConn := CreateConnectionStat("10.0.25.1", "2.2.2.2", 59782, 8000, TCP)
	AddIPTranslationToConnection(&DNatConn, "1.1.1.1", "10.0.25.1", 8000, 59782)
	localConn := CreateConnectionStat("1.1.1.1", "10.0.25.1", 8000, 59782, TCP)
	conns := slice.NewChain([]ConnectionStats{DNatConn, localConn})
	ns.determineConnectionIntraHost(conns)
	assert.True(t, conns.Get(0).IntraHost)
	assert.True(t, conns.Get(1).IntraHost)
}

func TestSNATIntraHost(t *testing.T) {
	ns := networkState{}
	SNatConn := CreateConnectionStat("2.2.2.2", "10.2.0.25", 59782, 8000, TCP)
	AddIPTranslationToConnection(&SNatConn, "10.2.0.25", "1.1.1.1", 8000, 6000)
	localConn := CreateConnectionStat("10.2.0.25", "2.2.2.2", 8000, 59782, TCP)
	conns := slice.NewChain([]ConnectionStats{SNatConn, localConn})
	ns.determineConnectionIntraHost(conns)
	assert.True(t, conns.Get(0).IntraHost)
	assert.True(t, conns.Get(1).IntraHost)
}

func CreateConnectionStat(source string, dest string, SPort uint16, DPort uint16, connType ConnectionType) ConnectionStats {
	return ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString(source),
		Dest:   util.AddressFromString(dest),
		SPort:  SPort,
		DPort:  DPort,
		Type:   connType,
	}}
}

func AddIPTranslationToConnection(conn *ConnectionStats, ReplSrcIP string, ReplDstIP string, ReplSrcPort uint16, ReplDstPort uint16) {
	translation := IPTranslation{
		ReplSrcIP:   util.AddressFromString(ReplSrcIP),
		ReplDstIP:   util.AddressFromString(ReplDstIP),
		ReplSrcPort: ReplSrcPort,
		ReplDstPort: ReplDstPort,
	}
	conn.IPTranslation = &translation
}
