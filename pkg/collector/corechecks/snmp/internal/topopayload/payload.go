// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package topopayload

// Device TODO
type Device struct {
	IP                    string `json:"ip"`
	Namespace             string `json:"namespace"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	ChassisIDType         string `json:"chassis_id_type"`
	ChassisID             string `json:"chassis_id"`
	CapabilitiesSupported string `json:"capabilities_supported"`
	CapabilitiesEnabled   string `json:"capabilities_enabled"`
}

// Endpoint TODO
type Endpoint struct {
	Device    Device    `json:"device"`
	Interface Interface `json:"interface"`
}

// Interface TODO
type Interface struct {
	IDType      string `json:"id_type"`
	ID          string `json:"id"`
	Description string `json:"description"`
}

// Connection TODO
type Connection struct {
	Remote Endpoint `json:"remote"`
	Local  Endpoint `json:"local"`
}

// TopologyPayload TODO
type TopologyPayload struct {
	Host        string       `json:"host"` // Agent host
	Device      Device       `json:"device"`
	Connections []Connection `json:"connections"`
}
