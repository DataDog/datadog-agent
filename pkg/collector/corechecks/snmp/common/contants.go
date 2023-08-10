// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

// Package common TODO comment
package common

// MetadataDeviceResource is the device resource name
const MetadataDeviceResource = "device"

// SnmpIntegrationName is the name of the snmp integration
const SnmpIntegrationName = "snmp"

// SnmpExternalTagsSourceType is the source id used for external tags
const SnmpExternalTagsSourceType = "snmp"

// IfAdminStatus exported type should have comment or be unexported
type IfAdminStatus int

// This const block should have a comment or be unexported
const (
	// don't use underscores in Go names; const AdminStatus_Up should be AdminStatusUp
	AdminStatus_Up IfAdminStatus = 1
	// don't use underscores in Go names; const AdminStatus_Down should be AdminStatusDown
	AdminStatus_Down IfAdminStatus = 2
	// don't use underscores in Go names; const AdminStatus_Testing should be AdminStatusTesting
	AdminStatus_Testing IfAdminStatus = 3
)

// don't use underscores in Go names; var adminStatus_StringMap should be adminStatusStringMap
var adminStatus_StringMap = map[IfAdminStatus]string{
	AdminStatus_Up:      "up",
	AdminStatus_Down:    "down",
	AdminStatus_Testing: "testing",
}

// AsString exported method should have comment or be unexported
func (i IfAdminStatus) AsString() string {
	status, ok := adminStatus_StringMap[i]
	if !ok {
		return "unknown"
	}
	return status
}

// IfOperStatus exported type should have comment or be unexported
type IfOperStatus int

// This const block should have a comment or be unexported
const (
	// don't use underscores in Go names; const OperStatus_Up should be OperStatusUp
	OperStatus_Up IfOperStatus = 1
	// don't use underscores in Go names; const OperStatus_Down should be OperStatusDown
	OperStatus_Down IfOperStatus = 2
	// don't use underscores in Go names; const OperStatus_Testing should be OperStatusTesting
	OperStatus_Testing IfOperStatus = 3
	// don't use underscores in Go names; const OperStatus_Unknown should be OperStatusUnknown
	OperStatus_Unknown IfOperStatus = 4
	// don't use underscores in Go names; const OperStatus_Dormant should be OperStatusDormant
	OperStatus_Dormant IfOperStatus = 5
	// don't use underscores in Go names; const OperStatus_NotPresent should be OperStatusNotPresent
	OperStatus_NotPresent IfOperStatus = 6
	// don't use underscores in Go names; const OperStatus_LowerLayerDown should be OperStatusLowerLayerDown
	OperStatus_LowerLayerDown IfOperStatus = 7
)

// don't use underscores in Go names; var operStatus_StringMap should be operStatusStringMap
var operStatus_StringMap = map[IfOperStatus]string{
	OperStatus_Up:             "up",
	OperStatus_Down:           "down",
	OperStatus_Testing:        "testing",
	OperStatus_Unknown:        "unknown",
	OperStatus_Dormant:        "dormant",
	OperStatus_NotPresent:     "not_present",
	OperStatus_LowerLayerDown: "lower_layer_down",
}

// AsString exported method should have comment or be unexported
func (i IfOperStatus) AsString() string {
	status, ok := operStatus_StringMap[i]
	if !ok {
		return "unknown"
	}
	return status
}

// InterfaceStatus exported type should have comment or be unexported
type InterfaceStatus string

// This const block should have a comment or be unexported
const (
	// don't use underscores in Go names; const InterfaceStatus_Up should be InterfaceStatusUp
	InterfaceStatus_Up InterfaceStatus = "up"
	// don't use underscores in Go names; const InterfaceStatus_Down should be InterfaceStatusDown
	InterfaceStatus_Down InterfaceStatus = "down"
	// don't use underscores in Go names; const InterfaceStatus_Warning should be InterfaceStatusWarning
	InterfaceStatus_Warning InterfaceStatus = "warning"
	// don't use underscores in Go names; const InterfaceStatus_Off should be InterfaceStatusOff
	InterfaceStatus_Off InterfaceStatus = "off"
)
