// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Versa API client
package client

type Content interface {
	ApplianceLiteResponse
}

// ApplicanceResponse /versa/ncs-services/vnms/appliance/appliance/lite
type ApplianceLiteResponse struct {
	Appliances []ApplianceLite `json:"appliances"`
	TotalCount int             `json:"totalCount"`
}

// Appliance encapsulates metadata for appliances
type ApplianceLite struct {
	Name                    string      `json:"name"`
	UUID                    string      `json:"uuid"`
	LastUpdatedTime         string      `json:"last-updated-time"`
	PingStatus              string      `json:"ping-status"`
	SyncStatus              string      `json:"sync-status"`
	CreatedAt               string      `json:"createdAt"`
	YangCompatibility       string      `json:"yang-compatibility-status"`
	ServicesStatus          string      `json:"services-status"`
	OverallStatus           string      `json:"overall-status"`
	ControlStatus           string      `json:"controll-status"`
	PathStatus              string      `json:"path-status"`
	TemplateStatus          string      `json:"templateStatus"`
	OwnerOrg                string      `json:"ownerOrg"`
	Type                    string      `json:"type"`
	SngCount                int         `json:"sngCount"`
	SoftwareVersion         string      `json:"softwareVersion"`
	Connector               string      `json:"connector"`
	ConnectorType           string      `json:"connectorType"`
	BranchID                string      `json:"branchId"`
	Services                []string    `json:"services"`
	IPAddress               string      `json:"ipAddress"`
	Location                string      `json:"location"`
	StartTime               string      `json:"startTime"`
	StolenSuspected         bool        `json:"stolenSuspected"`
	Hardware                Hardware    `json:"Hardware"`
	SPack                   SPack       `json:"SPack"`
	OssPack                 OssPack     `json:"OssPack"`
	RefreshCycleCount       int         `json:"refreshCycleCount"`
	BranchMaintenanceMode   bool        `json:"branch-maintenance-mode"`
	ApplianceTags           []string    `json:"applianceTags"`
	LockDetails             LockDetails `json:"lockDetails"`
	Unreachable             bool        `json:"unreachable"`
	BranchInMaintenanceMode bool        `json:"branchInMaintenanceMode"`
	Nodes                   Nodes       `json:"nodes"`
	UcpeNodes               UcpeNodes   `json:"ucpe-nodes"`
}

// Hardware encapsulates hardware metadata for an appliance
type Hardware struct {
	Model                        string `json:"model"`
	CPUCores                     int    `json:"cpuCores"`
	Memory                       string `json:"memory"`
	LPM                          bool   `json:"lpm"`
	Fanless                      bool   `json:"fanless"`
	IntelQuickAssistAcceleration bool   `json:"intelQuickAssistAcceleration"`
	SerialNo                     string `json:"serialNo"`
	CPUCount                     int    `json:"cpuCount"`
	CPULoad                      int    `json:"cpuLoad"`
	InterfaceCount               int    `json:"interfaceCount"`
	PackageName                  string `json:"packageName"`
	SSD                          bool   `json:"ssd"`
}

// SPack encapsulates SPack metadata for an appliance
type SPack struct {
	SPackVersion string `json:"spackVersion"`
}

// OssPack encapsulates OssPack metadata for an appliance
type OssPack struct {
	OsspackVersion string `json:"osspackVersion"`
}

// LockDetails encapsulates lock metadata for an appliance
type LockDetails struct {
	User     string `json:"user"`
	LockType string `json:"lockType"`
}

// Nodes encapsulates node metadata for an appliance
type Nodes struct {
	NodeStatusList []interface{} `json:"nodeStatusList"`
}

// UcpeNodes encapsulates UCPE node metadata for an appliance
type UcpeNodes struct {
	UcpeNodeStatusList []interface{} `json:"ucpeNodeStatusList"`
}
