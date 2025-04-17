// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Versa API client
package client

// Content encapsulates the content types of the Versa API
type Content interface {
	[]Appliance |
		ControllerResponse |
		DirectorStatus |
		int // for row counts
}

// ControllerResponse /versa/ncs-services/vnms/dashboard/status/headEnds
type ControllerResponse struct {
	ControllerStatuses []ControllerStatus `json:"applianceStatus"`
	HAStatus           ControllerHAStatus `json:"haStatus"`
}

// HAStatus encapsulates HA status metadata for a headend
type ControllerHAStatus struct {
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

type Appliance struct {
	Name                    string              `json:"name"`
	UUID                    string              `json:"uuid"`
	ApplianceLocation       ApplianceLocation   `json:"applianceLocation"`
	LastUpdatedTime         string              `json:"last-updated-time"`
	PingStatus              string              `json:"ping-status"`
	SyncStatus              string              `json:"sync-status"`
	YangCompatibilityStatus string              `json:"yang-compatibility-status"`
	ServicesStatus          string              `json:"services-status"`
	OverallStatus           string              `json:"overall-status"`
	ControllStatus          string              `json:"controll-status"`
	PathStatus              string              `json:"path-status"`
	IntraChassisHAStatus    HAStatus            `json:"intra-chassis-ha-status"`
	InterChassisHAStatus    HAStatus            `json:"inter-chassis-ha-status"`
	TemplateStatus          string              `json:"templateStatus"`
	OwnerOrgUUID            string              `json:"ownerOrgUuid"`
	OwnerOrg                string              `json:"ownerOrg"`
	Type                    string              `json:"type"`
	SngCount                int                 `json:"sngCount"`
	SoftwareVersion         string              `json:"softwareVersion"`
	BranchID                string              `json:"branchId"`
	Services                []string            `json:"services"`
	IPAddress               string              `json:"ipAddress"`
	StartTime               string              `json:"startTime"`
	StolenSuspected         bool                `json:"stolenSuspected"`
	Hardware                Hardware            `json:"Hardware"`
	SPack                   SPack               `json:"SPack"`
	OssPack                 OssPack             `json:"OssPack"`
	AppIDDetails            AppIDDetails        `json:"appIdDetails"`
	AlarmSummary            Table               `json:"alarmSummary"`
	CPEHealth               Table               `json:"cpeHealth"`
	ApplicationStats        Table               `json:"applicationStats"`
	PolicyViolation         Table               `json:"policyViolation"`
	RefreshCycleCount       int                 `json:"refreshCycleCount"`
	SubType                 string              `json:"subType"`
	BranchMaintenanceMode   bool                `json:"branch-maintenance-mode"`
	ApplianceTags           []string            `json:"applianceTags"`
	ApplianceCapabilities   CapabilitiesWrapper `json:"applianceCapabilities"`
	Unreachable             bool                `json:"unreachable"`
	BranchInMaintenanceMode bool                `json:"branchInMaintenanceMode"`
	Nodes                   Nodes               `json:"nodes"`
	UcpeNodes               UcpeNodes           `json:"ucpe-nodes"`
}

type ApplianceLocation struct {
	ApplianceName string `json:"applianceName"`
	ApplianceUUID string `json:"applianceUuid"`
	LocationID    string `json:"locationId"`
	Latitude      string `json:"latitude"`
	Longitude     string `json:"longitude"`
	Type          string `json:"type"`
}

type HAStatus struct {
	HAConfigured bool `json:"ha-configured"`
}

type Hardware struct {
	Name                         string `json:"name"`
	Model                        string `json:"model"`
	CPUCores                     int    `json:"cpuCores"`
	Memory                       string `json:"memory"`
	FreeMemory                   string `json:"freeMemory"`
	DiskSize                     string `json:"diskSize"`
	FreeDisk                     string `json:"freeDisk"`
	LPM                          bool   `json:"lpm"`
	Fanless                      bool   `json:"fanless"`
	IntelQuickAssistAcceleration bool   `json:"intelQuickAssistAcceleration"`
	FirmwareVersion              string `json:"firmwareVersion"`
	Manufacturer                 string `json:"manufacturer"`
	SerialNo                     string `json:"serialNo"`
	HardWareSerialNo             string `json:"hardWareSerialNo"`
	CPUModel                     string `json:"cpuModel"`
	CPUCount                     int    `json:"cpuCount"`
	CPULoad                      int    `json:"cpuLoad"`
	InterfaceCount               int    `json:"interfaceCount"`
	PackageName                  string `json:"packageName"`
	SKU                          string `json:"sku"`
	SSD                          bool   `json:"ssd"`
}

type SPack struct {
	Name         string `json:"name"`
	SPackVersion string `json:"spackVersion"`
	APIVersion   string `json:"apiVersion"`
	Flavor       string `json:"flavor"`
	ReleaseDate  string `json:"releaseDate"`
	UpdateType   string `json:"updateType"`
}

type OssPack struct {
	Name           string `json:"name"`
	OssPackVersion string `json:"osspackVersion"`
	UpdateType     string `json:"updateType"`
}

type AppIDDetails struct {
	AppIDInstalledEngineVersion string `json:"appIdInstalledEngineVersion"`
	AppIDInstalledBundleVersion string `json:"appIdInstalledBundleVersion"`
}

type Table struct {
	TableID     string     `json:"tableId"`
	TableName   string     `json:"tableName"`
	MonitorType string     `json:"monitorType"`
	ColumnNames []string   `json:"columnNames"`
	Rows        []TableRow `json:"rows"`
}

type TableRow struct {
	FirstColumnValue string        `json:"firstColumnValue"`
	ColumnValues     []interface{} `json:"columnValues"`
}

type CapabilitiesWrapper struct {
	Capabilities []string `json:"capabilities"`
}

type Nodes struct {
	NodeStatusList NodeStatus `json:"nodeStatusList"`
}

type NodeStatus struct {
	VMName     string `json:"vm-name"`
	VMStatus   string `json:"vm-status"`
	NodeType   string `json:"node-type"`
	HostIP     string `json:"host-ip"`
	CPULoad    int    `json:"cpu-load"`
	MemoryLoad int    `json:"memory-load"`
	LoadFactor int    `json:"load-factor"`
	SlotID     int    `json:"slot-id"`
}

type UcpeNodes struct {
	UcpeNodeStatusList []interface{} `json:"ucpeNodeStatusList"`
}
