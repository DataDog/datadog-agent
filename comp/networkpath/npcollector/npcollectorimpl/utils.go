// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"slices"

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

func getDomain(conns *model.Connections, ip string) string {
	var domain string
	if dnsEntry := conns.Dns[ip]; dnsEntry != nil && len(dnsEntry.Names) > 0 {
		domain = dnsEntry.Names[0]
	}
	if !slices.Contains(conns.Domains, domain) {
		// TODO: Check why conns.Domains does not contains all domains in conns.Dns
		//       Example, those ones:
		//         "2600:1f18:24e6:b902:a9be:cf94:3d31:e987": {
		//            "names": [
		//                "llmobs-intake.datadoghq.com",
		//                "ndmflow-intake.datadoghq.com",
		//                "sbom-intake.datadoghq.com",
		//                "netpath-intake.datadoghq.com",
		//                "instrumentation-telemetry-intake.datadoghq.com",
		//                "contimage-intake.datadoghq.com",
		//                "ndm-intake.datadoghq.com",
		//                "snmp-traps-intake.datadoghq.com",
		//                "contlcycle-intake.datadoghq.com",
		//                "event-platform-intake.datadoghq.com"
		//            ]
		//        },
		domain = ""
	}
	return domain
}
