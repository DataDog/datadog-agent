// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"net"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-go/v5/statsd"
)

func shouldScheduleNetworkPathForConn(conn *model.Connection, networkID string, statsdClient statsd.ClientInterface) bool {
	// TODO: REFACTOR AS METHOD OF NPCOLLECTOR to make it easier to use statsdClient ?
	if conn == nil || conn.Direction != model.ConnectionDirection_outgoing {
		return false
	}
	remoteIP := net.ParseIP(conn.Raddr.Ip)
	if remoteIP.IsLoopback() || conn.IntraHost {
		statsdClient.Incr(networkPathCollectorMetricPrefix+"schedule.skipped", []string{"reason:skip_loopback"}, 1) //nolint:errcheck
		return false
	}
	if conn.RemoteNetworkId != "" && conn.RemoteNetworkId == networkID {
		statsdClient.Incr(networkPathCollectorMetricPrefix+"schedule.skipped", []string{"reason:skip_same_network_id"}, 1) //nolint:errcheck
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
