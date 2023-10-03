// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// SnmpIntegrationName is the name of the snmp integration
const SnmpIntegrationName = "snmp"

// SnmpExternalTagsSourceType is the source id used for external tags
const SnmpExternalTagsSourceType = "snmp"

type IfAdminStatus int

const (
	AdminStatus_Up      IfAdminStatus = 1
	AdminStatus_Down    IfAdminStatus = 2
	AdminStatus_Testing IfAdminStatus = 3
)

var adminStatus_StringMap map[IfAdminStatus]string = map[IfAdminStatus]string{
	AdminStatus_Up:      "up",
	AdminStatus_Down:    "down",
	AdminStatus_Testing: "testing",
}

func (i IfAdminStatus) AsString() string {
	status, ok := adminStatus_StringMap[i]
	if !ok {
		return "unknown"
	}
	return status
}

type IfOperStatus int

const (
	OperStatus_Up             IfOperStatus = 1
	OperStatus_Down           IfOperStatus = 2
	OperStatus_Testing        IfOperStatus = 3
	OperStatus_Unknown        IfOperStatus = 4
	OperStatus_Dormant        IfOperStatus = 5
	OperStatus_NotPresent     IfOperStatus = 6
	OperStatus_LowerLayerDown IfOperStatus = 7
)

var operStatus_StringMap map[IfOperStatus]string = map[IfOperStatus]string{
	OperStatus_Up:             "up",
	OperStatus_Down:           "down",
	OperStatus_Testing:        "testing",
	OperStatus_Unknown:        "unknown",
	OperStatus_Dormant:        "dormant",
	OperStatus_NotPresent:     "not_present",
	OperStatus_LowerLayerDown: "lower_layer_down",
}

func (i IfOperStatus) AsString() string {
	status, ok := operStatus_StringMap[i]
	if !ok {
		return "unknown"
	}
	return status
}

type InterfaceStatus string

const (
	InterfaceStatus_Up      InterfaceStatus = "up"
	InterfaceStatus_Down    InterfaceStatus = "down"
	InterfaceStatus_Warning InterfaceStatus = "warning"
	InterfaceStatus_Off     InterfaceStatus = "off"
)
