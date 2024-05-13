// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package lldp

// ChassisIDSubtypeMap mapping to translate into human-readable value for LldpChassisIdSubtype
// Ref http://www.mibdepot.com/cgi-bin/getmib3.cgi?win=mib_a&i=1&n=LLDP-MIB&r=cisco&f=LLDP-MIB-V1SMI.my&v=v1&t=def#LldpChassisIdSubtype
var ChassisIDSubtypeMap = map[string]string{
	"1": "chassis_component",
	"2": "interface_alias",
	"3": "port_component",
	"4": "mac_address",
	"5": "network_address",
	"6": "interface_name",
	"7": "local",
}

// PortIDSubTypeMap mapping to translate into human-readable value for LldpPortIdSubtype
// Ref http://www.mibdepot.com/cgi-bin/getmib3.cgi?win=mib_a&i=1&n=LLDP-MIB&r=cisco&f=LLDP-MIB-V1SMI.my&v=v1&t=def#LldpPortIdSubtype
var PortIDSubTypeMap = map[string]string{
	"1": "interface_alias",
	"2": "port_component",
	"3": "mac_address",
	"4": "network_address",
	"5": "interface_name",
	"6": "agent_circuit_id",
	"7": "local",
}
