// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func convertProtocol(connType model.ConnectionType) payload.Protocol {
	if connType == model.ConnectionType_tcp {
		return payload.ProtocolTCP
	} else if connType == model.ConnectionType_udp {
		return payload.ProtocolUDP
	}
	return ""
}

func getDNSNameForIP(conns *model.Connections, ip string) string {
	var domain string
	if dnsEntry := conns.Dns[ip]; dnsEntry != nil && len(dnsEntry.Names) > 0 {
		// We are only using the first entry for now, but in the future, if we find a good solution,
		// we might want to report the other DNS names too if necessary (need more investigation on how to best achieve that).
		domain = dnsEntry.Names[0]
	}
	return domain
}
