// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Versa API client
package client

// Content encapsulates the content types of the Versa API
type Content interface {
	ApplianceLiteResponse |
		ControllerResponse |
		DirectorStatus
}

// ApplianceLiteResponse /versa/ncs-services/vnms/appliance/appliance/lite
type ApplianceLiteResponse struct {
	Appliances []ApplianceLite `json:"appliances"`
	TotalCount int             `json:"totalCount"`
}

// ApplianceLite encapsulates metadata for appliances
type ApplianceLite struct {
	Name              string `json:"name"`
	UUID              string `json:"uuid"`
	LastUpdatedTime   string `json:"last-updated-time"`
	PingStatus        string `json:"ping-status"`
	SyncStatus        string `json:"sync-status"`
	CreatedAt         string `json:"createdAt"`
	YangCompatibility string `json:"yang-compatibility-status"`
	ServicesStatus    string `json:"services-status"`
	OverallStatus     string `json:"overall-status"`
	// ControlStatus is a misspelled field in the API
	ControlStatus           string      `json:"controll-status"` //nolint:misspell
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

// ControllerResponse /versa/ncs-services/vnms/dashboard/status/headEnds
type ControllerResponse struct {
	ControllerStatuses []ControllerStatus `json:"applianceStatus"`
	HAStatus           HAStatus           `json:"haStatus"`
}

// HAStatus encapsulates HA status metadata for a headend
type HAStatus struct {
	Enabled bool `json:"enabled"`
}

// ControllerStatus encapsulates metadata for a controller
type ControllerStatus struct {
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	Status     string `json:"status"`
	LockStatus string `json:"lockStatus"`
	SyncStatus string `json:"syncStatus"`
	IPAddress  string `json:"ip-address"`
}

// DirectorStatus /versa/ncs-services/vnms/dashboard/vdStatus
type DirectorStatus struct {
	HAConfig struct {
		ClusterID                      string   `json:"clusterid"`
		FailoverTimeout                int      `json:"failoverTimeout"`
		SlaveStartTimeout              int      `json:"slaveStartTimeout"`
		AutoSwitchOverTimeout          int      `json:"autoSwitchOverTimeout"`
		AutoSwitchOverEnabled          bool     `json:"autoSwitchOverEnabled"`
		DesignatedMaster               bool     `json:"designatedMaster"`
		StartupMode                    string   `json:"startupMode"`
		MyVnfManagementIPs             []string `json:"myVnfManagementIps"`
		VDSBInterfaces                 []string `json:"vdsbinterfaces"`
		StartupModeHA                  bool     `json:"startupModeHA"`
		MyNcsHaSetAsMaster             bool     `json:"myNcsHaSet"`
		PingViaAnyDeviceSuccessful     bool     `json:"pingViaAnyDeviceSuccessful"`
		PeerReachableViaNcsPortDevices bool     `json:"peerReachableViaNcsPortAndDevices"`
		HAEnabledOnBothNodes           bool     `json:"haEnabledOnBothNodes"`
	} `json:"haConfig"`
	HADetails struct {
		Enabled           bool `json:"enabled"`
		DesignatedMaster  bool `json:"designatedMaster"`
		PeerVnmsHaDetails []struct {
		} `json:"peerVnmsHaDetails"`
		EnableHaInProgress bool `json:"enableHaInProgress"`
	} `json:"haDetails"`
	VDSBInterfaces []string `json:"vdSBInterfaces"`
	SystemDetails  struct {
		CPUCount   int    `json:"cpuCount"`
		CPULoad    string `json:"cpuLoad"`
		Memory     string `json:"memory"`
		MemoryFree string `json:"memoryFree"`
		Disk       string `json:"disk"`
		DiskUsage  string `json:"diskUsage"`
	} `json:"systemDetails"`
	PkgInfo struct {
		Version     string `json:"version"`
		PackageDate string `json:"packageDate"`
		Name        string `json:"name"`
		PackageID   string `json:"packageId"`
		UIPackageID string `json:"uiPackageId"`
		Branch      string `json:"branch"`
	} `json:"pkgInfo"`
	SystemUpTime struct {
		CurrentTime       string `json:"currentTime"`
		ApplicationUpTime string `json:"applicationUpTime"`
		SysProcUptime     string `json:"sysProcUptime"`
		SysUpTimeDetail   string `json:"sysUpTimeDetail"`
	} `json:"systemUpTime"`
}
