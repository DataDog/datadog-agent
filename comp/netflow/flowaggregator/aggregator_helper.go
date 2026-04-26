// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import model "github.com/DataDog/agent-payload/v5/process"

func netflowProtocolToConnectionType(ipProtocol uint32) (model.ConnectionType, bool) {
	switch ipProtocol {
	case 6:
		return model.ConnectionType_tcp, true
	case 17:
		return model.ConnectionType_udp, true
	default:
		return 0, false
	}
}

func toUint16Port(port int32) (uint16, bool) {
	if port < 0 || port > 65535 {
		return 0, false
	}
	return uint16(port), true
}
