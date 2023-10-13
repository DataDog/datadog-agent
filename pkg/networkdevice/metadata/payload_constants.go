// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package metadata

// IfAdminStatus interface admin status type
type IfAdminStatus int

// IfAdminStatus enums
const (
	AdminStatusUp      IfAdminStatus = 1
	AdminStatusDown    IfAdminStatus = 2
	AdminStatusTesting IfAdminStatus = 3
)

var adminStatusStringMap = map[IfAdminStatus]string{
	AdminStatusUp:      "up",
	AdminStatusDown:    "down",
	AdminStatusTesting: "testing",
}

// AsString convert to string value
func (i IfAdminStatus) AsString() string {
	status, ok := adminStatusStringMap[i]
	if !ok {
		return "unknown"
	}
	return status
}

// IfOperStatus interface oper status type
type IfOperStatus int

// IfOperStatus enums
const (
	OperStatusUp             IfOperStatus = 1
	OperStatusDown           IfOperStatus = 2
	OperStatusTesting        IfOperStatus = 3
	OperStatusUnknown        IfOperStatus = 4
	OperStatusDormant        IfOperStatus = 5
	OperStatusNotPresent     IfOperStatus = 6
	OperStatusLowerLayerDown IfOperStatus = 7
)

var operStatusStringMap = map[IfOperStatus]string{
	OperStatusUp:             "up",
	OperStatusDown:           "down",
	OperStatusTesting:        "testing",
	OperStatusUnknown:        "unknown",
	OperStatusDormant:        "dormant",
	OperStatusNotPresent:     "not_present",
	OperStatusLowerLayerDown: "lower_layer_down",
}

// AsString convert to string value
func (i IfOperStatus) AsString() string {
	status, ok := operStatusStringMap[i]
	if !ok {
		return "unknown"
	}
	return status
}

// InterfaceStatus interface status
type InterfaceStatus string

// InterfaceStatus enums
const (
	InterfaceStatusUp      InterfaceStatus = "up"
	InterfaceStatusDown    InterfaceStatus = "down"
	InterfaceStatusWarning InterfaceStatus = "warning"
	InterfaceStatusOff     InterfaceStatus = "off"
)
