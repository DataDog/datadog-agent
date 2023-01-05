// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// MetadataDeviceResource is the device resource name
const MetadataDeviceResource = "device"

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

type InterfaceStatus string

const (
	InterfaceStatus_Up      InterfaceStatus = "up"
	InterfaceStatus_Down    InterfaceStatus = "down"
	InterfaceStatus_Warning InterfaceStatus = "warning"
	InterfaceStatus_Off     InterfaceStatus = "off"
)
