// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"

// PayloadMetadataBatchSize is the number of resources per event payload
// Resources are devices, interfaces, etc
const PayloadMetadataBatchSize = 100

// DeviceStatus enum type
type DeviceStatus int32

const (
	// DeviceStatusReachable means the device can be reached by snmp integration
	DeviceStatusReachable = DeviceStatus(1)
	// DeviceStatusUnreachable means the device cannot be reached by snmp integration
	DeviceStatusUnreachable = DeviceStatus(2)
)

type IDType string

const (
	// IDTypeMacAddress represent mac address in `00:00:00:00:00:00` format
	IDTypeMacAddress     = "mac_address"
	IDTypeInterfaceName  = "interface_name"
	IDTypeInterfaceAlias = "interface_alias"
)

// NetworkDevicesMetadata contains network devices metadata
type NetworkDevicesMetadata struct {
	Subnet           string                 `json:"subnet,omitempty"`
	Namespace        string                 `json:"namespace"`
	Devices          []DeviceMetadata       `json:"devices,omitempty"`
	Interfaces       []InterfaceMetadata    `json:"interfaces,omitempty"`
	IPAddresses      []IPAddressMetadata    `json:"ip_addresses,omitempty"`
	Links            []TopologyLinkMetadata `json:"links,omitempty"`
	NetflowExporters []NetflowExporter      `json:"netflow_exporters,omitempty"`
	Diagnoses        []DiagnosisMetadata    `json:"diagnoses,omitempty"`
	CollectTimestamp int64                  `json:"collect_timestamp"`
}

// DeviceMetadata contains device metadata
type DeviceMetadata struct {
	ID           string       `json:"id"`
	IDTags       []string     `json:"id_tags"` // id_tags is the input to produce device.id, it's also used to correlated with device metrics.
	Tags         []string     `json:"tags"`
	IPAddress    string       `json:"ip_address"`
	Status       DeviceStatus `json:"status"`
	Name         string       `json:"name,omitempty"`
	Description  string       `json:"description,omitempty"`
	SysObjectID  string       `json:"sys_object_id,omitempty"`
	Location     string       `json:"location,omitempty"`
	Profile      string       `json:"profile,omitempty"`
	Vendor       string       `json:"vendor,omitempty"`
	Subnet       string       `json:"subnet,omitempty"`
	SerialNumber string       `json:"serial_number,omitempty"`
	Version      string       `json:"version,omitempty"`
	ProductName  string       `json:"product_name,omitempty"`
	Model        string       `json:"model,omitempty"`
	OsName       string       `json:"os_name,omitempty"`
	OsVersion    string       `json:"os_version,omitempty"`
	OsHostname   string       `json:"os_hostname,omitempty"`
	Integration  string       `json:"integration,omitempty"` // indicates the source of the data SNMP, meraki_api, etc.
}

// InterfaceMetadata contains interface metadata
type InterfaceMetadata struct {
	DeviceID      string               `json:"device_id"`
	IDTags        []string             `json:"id_tags"` // used to correlate with interface metrics
	Index         int32                `json:"index"`   // IF-MIB ifIndex type is InterfaceIndex (Integer32 (1..2147483647))
	Name          string               `json:"name,omitempty"`
	Alias         string               `json:"alias,omitempty"`
	Description   string               `json:"description,omitempty"`
	MacAddress    string               `json:"mac_address,omitempty"`
	AdminStatus   common.IfAdminStatus `json:"admin_status,omitempty"`   // IF-MIB ifAdminStatus type is INTEGER
	OperStatus    common.IfOperStatus  `json:"oper_status,omitempty"`    // IF-MIB ifOperStatus type is INTEGER
	MerakiEnabled *bool                `json:"meraki_enabled,omitempty"` // enabled bool for Meraki devices, use a pointer to determine if the value was actually sent
	MerakiStatus  string               `json:"meraki_status,omitempty"`  // status for Meraki devices
}

// IPAddressMetadata contains ip address metadata
type IPAddressMetadata struct {
	InterfaceID string `json:"interface_id"`
	IPAddress   string `json:"ip_address"`
	Prefixlen   int32  `json:"prefixlen,omitempty"`
}

// TopologyLinkDevice contain device link data
type TopologyLinkDevice struct {
	DDID        string `json:"dd_id,omitempty"`
	ID          string `json:"id,omitempty"`
	IDType      string `json:"id_type,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
}

// TopologyLinkInterface contain interface link data
type TopologyLinkInterface struct {
	DDID        string `json:"dd_id,omitempty"`
	ID          string `json:"id"`
	IDType      string `json:"id_type,omitempty"`
	Description string `json:"description,omitempty"`
}

// TopologyLinkSide contain data for remote or local side of the link
type TopologyLinkSide struct {
	Device    *TopologyLinkDevice    `json:"device,omitempty"`
	Interface *TopologyLinkInterface `json:"interface,omitempty"`
}

// TopologyLinkMetadata contains topology interface to interface links metadata
type TopologyLinkMetadata struct {
	ID         string            `json:"id"`
	SourceType string            `json:"source_type"`
	Local      *TopologyLinkSide `json:"local"`
	Remote     *TopologyLinkSide `json:"remote"`
}

// NetflowExporter contains netflow exporters info
type NetflowExporter struct {
	ID        string `json:"id"` // used by backend as unique id (e.g. in cache)
	IPAddress string `json:"ip_address"`
	FlowType  string `json:"flow_type"`
}

// Diagnosis contain data for a diagnosis
type Diagnosis struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Code     string `json:"code"`
}

// DiagnosisMetadata contains diagnoses info
type DiagnosisMetadata struct {
	ResourceType string      `json:"resource_type"`
	ResourceID   string      `json:"resource_id"`
	Diagnoses    []Diagnosis `json:"diagnoses"`
}
