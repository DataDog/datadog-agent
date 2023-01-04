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
	AdminStatus_Up IfAdminStatus = iota + 1
	AdminStatus_Down
	AdminStatus_Testing
)

type IfOperStatus int

const (
	OperStatus_Up IfOperStatus = iota + 1
	OperStatus_Down
	OperStatus_Testing
	OperStatus_Unknown
	OperStatus_Dormant
	OperStatus_NotPresent
	OperStatus_LowerLayerDown
)
