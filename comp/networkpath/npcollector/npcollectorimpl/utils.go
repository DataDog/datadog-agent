// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"net"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func shouldScheduleNetworkPathForConn(conn *model.Connection) bool {
	if conn == nil || conn.Direction != model.ConnectionDirection_outgoing {
		return false
	}
	remoteIP := net.ParseIP(conn.Raddr.Ip)
	if remoteIP.IsLoopback() {
		return false
	}
	return conn.Family == model.ConnectionFamily_v4
}

func convertProtocol(connType model.ConnectionType) payload.Protocol {
	if connType == model.ConnectionType_tcp {
		return payload.ProtocolTCP
	} else if connType == model.ConnectionType_udp {
		return payload.ProtocolUDP
	}
	return ""
}
