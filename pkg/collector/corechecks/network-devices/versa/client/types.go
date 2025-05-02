// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Versa API client
package client

// Content encapsulates the content types of the Versa API
type Content interface {
	[]Appliance |
		int | // for row counts
		[]TenantConfig |
		OrganizationListResponse |
		DirectorStatus
}

// DirectorStatus /versa/ncs-services/vnms/dashboard/vdStatus
type DirectorStatus struct {
	HAConfig       DirectorHAConfig      `json:"haConfig"`
	HADetails      DirectorHADetails     `json:"haDetails"`
	VDSBInterfaces []string              `json:"vdSBInterfaces"`
	SystemDetails  DirectorSystemDetails `json:"systemDetails"`
	PkgInfo        DirectorPkgInfo       `json:"pkgInfo"`
	SystemUpTime   DirectorSystemUpTime  `json:"systemUpTime"`
}

// DirectorHAConfig encapsulates metadata for a Versa director's HA configuration
type DirectorHAConfig struct {
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
	MyNcsHaSetAsMaster             bool     `json:"myNcsHaSetAsMaster"`
	PingViaAnyDeviceSuccessful     bool     `json:"pingViaAnyDeviceSuccessful"`
	PeerReachableViaNcsPortDevices bool     `json:"peerReachableViaNcsPortAndDevices"`
	HAEnabledOnBothNodes           bool     `json:"haEnabledOnBothNodes"`
}

// DirectorHADetails encapsulates metadata for a Versa director's HA details
type DirectorHADetails struct {
	Enabled           bool `json:"enabled"`
	DesignatedMaster  bool `json:"designatedMaster"`
	PeerVnmsHaDetails []struct {
	} `json:"peerVnmsHaDetails"`
	EnableHaInProgress bool `json:"enableHaInProgress"`
}

// DirectorSystemDetails encapsulates metadata for a Versa director's system details
type DirectorSystemDetails struct {
	CPUCount   int    `json:"cpuCount"`
	CPULoad    string `json:"cpuLoad"`
	Memory     string `json:"memory"`
	MemoryFree string `json:"memoryFree"`
	Disk       string `json:"disk"`
	DiskUsage  string `json:"diskUsage"`
}

// DirectorPkgInfo encapsulates metadata for a Versa director's package information
type DirectorPkgInfo struct {
	Version     string `json:"version"`
	PackageDate string `json:"packageDate"`
	Name        string `json:"name"`
	PackageID   string `json:"packageId"`
	UIPackageID string `json:"uiPackageId"`
	Branch      string `json:"branch"`
}

// DirectorSystemUpTime encapsulates metadata for a Versa director's system uptime
type DirectorSystemUpTime struct {
	CurrentTime       string `json:"currentTime"`
	ApplicationUpTime string `json:"applicationUpTime"`
	SysProcUptime     string `json:"sysProcUptime"`
	SysUpTimeDetail   string `json:"sysUpTimeDetail"`
}

// Appliance encapsulates metadata and some metrics for a Versa appliance
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

// ApplianceLocation encapsulates metadata for an appliance location
type ApplianceLocation struct {
	ApplianceName string `json:"applianceName"`
	ApplianceUUID string `json:"applianceUuid"`
	LocationID    string `json:"locationId"`
	Latitude      string `json:"latitude"`
	Longitude     string `json:"longitude"`
	Type          string `json:"type"`
}

// HAStatus encapsulates metadata for an appliance's HA status
type HAStatus struct {
	HAConfigured bool `json:"ha-configured"`
}

// Hardware encapsulates hardware metadata for a Versa appliance
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

// SPack encapsulates metadata for a Versa SPack
type SPack struct {
	Name         string `json:"name"`
	SPackVersion string `json:"spackVersion"`
	APIVersion   string `json:"apiVersion"`
	Flavor       string `json:"flavor"`
	ReleaseDate  string `json:"releaseDate"`
	UpdateType   string `json:"updateType"`
}

// OssPack encapsulates metadata for a Versa OSS pack
type OssPack struct {
	Name           string `json:"name"`
	OssPackVersion string `json:"osspackVersion"`
	UpdateType     string `json:"updateType"`
}

// AppIDDetails encapsulates metadata for a Versa AppID
type AppIDDetails struct {
	AppIDInstalledEngineVersion string `json:"appIdInstalledEngineVersion"`
	AppIDInstalledBundleVersion string `json:"appIdInstalledBundleVersion"`
}

// Table encapsulates metadata for a Versa table
type Table struct {
	TableID     string     `json:"tableId"`
	TableName   string     `json:"tableName"`
	MonitorType string     `json:"monitorType"`
	ColumnNames []string   `json:"columnNames"`
	Rows        []TableRow `json:"rows"`
}

// TableRow encapsulates metadata for a row in a Versa table
type TableRow struct {
	FirstColumnValue string        `json:"firstColumnValue"`
	ColumnValues     []interface{} `json:"columnValues"`
}

// CapabilitiesWrapper encapsulates metadata for a Versa appliance's capabilities
type CapabilitiesWrapper struct {
	Capabilities []string `json:"capabilities"`
}

// Nodes encapsulates metadata for a Versa appliance's nodes
type Nodes struct {
	NodeStatusList NodeStatus `json:"nodeStatusList"`
}

// NodeStatus encapsulates metadata for a node in a Versa appliance
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

// UcpeNodes encapsulates metadata for a Versa appliance's UCPE nodes
type UcpeNodes struct {
	UcpeNodeStatusList []interface{} `json:"ucpeNodeStatusList"`
}

// TenantConfig encapsulates metadata for a Versa tenant
type TenantConfig struct {
	Name                     string              `json:"name"`
	UUID                     string              `json:"uuid"`
	Parent                   string              `json:"parent"`
	SubscriptionPlan         string              `json:"subscriptionPlan"`
	Description              string              `json:"description"`
	ID                       int                 `json:"id"`
	AuthType                 string              `json:"authType"`
	VsaBasicUsers            int                 `json:"vsaBasicUsers"`
	VsaAdvancedUsers         int                 `json:"vsaAdvancedUsers"`
	VsaBasicLicensePeriod    int                 `json:"vsaBasicLicensePeriod"`
	VsaAdvancedLicensePeriod int                 `json:"vsaAdvancedLicensePeriod"`
	CpeDeploymentType        string              `json:"cpeDeploymentType"`
	Appliances               []ApplianceEntry    `json:"appliances"`
	VrfsGroups               []VRFGroup          `json:"vrfsGroups"`
	WanNetworkGroups         []WANNetworkGroup   `json:"wanNetworkGroups"`
	PushCaConfig             bool                `json:"pushCaConfig"`
	SharedControlPlane       bool                `json:"sharedControlPlane"`
	DynamicTenantConfig      DynamicTenantConfig `json:"dynamicTenantConfig"`
	BlockInterRegionRouting  bool                `json:"blockInterRegionRouting"`
	ApplianceTags            []string            `json:"appliance-tags"`
	Connectors               []string            `json:"connectors"`
}

// ApplianceEntry encapsulates metadata for an appliance entry in a tenant
// TODO: maybe drop this, we call for appliances separately anyway
type ApplianceEntry struct {
	ApplianceUUID string        `json:"applianceuuid"`
	CustomParams  []interface{} `json:"customParams"` // can be refined if needed
}

// VRFGroup encapsulates metadata for a VRF group in a tenant
type VRFGroup struct {
	ID          int    `json:"id"`
	VrfID       int    `json:"vrfId"`
	Name        string `json:"name"`
	Description string `json:"description"` // using plain string here
	EnableVPN   bool   `json:"enable_vpn"`
}

// WANNetworkGroup encapsulates metadata for a WAN network group in a tenant
type WANNetworkGroup struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"` // using plain string
	TransportDomains []string `json:"transport-domains"`
}

// DynamicTenantConfig encapsulates metadata for a dynamic tenant configuration
type DynamicTenantConfig struct {
	UUID               string `json:"uuid"`
	InactivityInterval int    `json:"inactivityInterval"`
}

// OrganizationListResponse encapsulates the response for a list of organizations
type OrganizationListResponse struct {
	TotalCount    int            `json:"totalCount"`
	Organizations []Organization `json:"organizations"`
}

// Organization encapsulates metadata for a Versa organization
type Organization struct {
	UUID                    string   `json:"uuid"`
	Name                    string   `json:"name"`
	ParentOrg               string   `json:"paraentOrg"` // Keeping the JSON key as is, there's a typo in the API
	Connectors              []string `json:"connectors"`
	Plan                    string   `json:"plan"`
	GlobalOrgID             string   `json:"globalOrgId"`
	Description             string   `json:"description"`
	SharedControlPlane      bool     `json:"sharedControlPlane"`
	BlockInterRegionRouting bool     `json:"blockInterRegionRouting"`
	CpeDeploymentType       string   `json:"cpeDeploymentType"`
	AuthType                string   `json:"authType"`
	ProviderOrg             bool     `json:"providerOrg"`
	Depth                   int      `json:"depth"`
	PushCaConfig            bool     `json:"pushCaConfig"`
}
