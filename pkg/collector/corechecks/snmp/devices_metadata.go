package snmp

// Scalar OIDs
var sysNameOID = "1.3.6.1.2.1.1.5.0"
var sysDescrOID = "1.3.6.1.2.1.1.1.0"
var sysObjectIDOID = "1.3.6.1.2.1.1.2.0"

// TODO: TEST ME
var metadataScalarOIDs = []string{
	sysNameOID,
	sysDescrOID,
	sysObjectIDOID,
}

// Column OIDs
var ifNameOID = "1.3.6.1.2.1.31.1.1.1.1"
var ifAliasOID = "1.3.6.1.2.1.31.1.1.1.18"
var ifDescrOID = "1.3.6.1.2.1.2.2.1.2"
var ifPhysAddressOID = "1.3.6.1.2.1.2.2.1.6"
var ifAdminStatusOID = "1.3.6.1.2.1.2.2.1.7"
var ifOperStatusOID = "1.3.6.1.2.1.2.2.1.8"

// TODO: TEST ME
var metadataColumnOIDs = []string{
	ifNameOID,
	ifAliasOID,
	ifDescrOID,
	ifPhysAddressOID,
	ifAdminStatusOID,
	ifOperStatusOID,
}

type NetworkDevicesMetadata struct {
	Subnet     string              `json:"subnet"`
	Devices    []DeviceMetadata    `json:"devices"`
	Interfaces []InterfaceMetadata `json:"interfaces"`
}

type DeviceMetadata struct {
	Id          string   `json:"id"`
	IdTags      []string `json:"id_tags"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IpAddress   string   `json:"ip_address"`
	SysObjectId string   `json:"sys_object_id"`
	Profile     string   `json:"profile"`
	Vendor      string   `json:"vendor"`
	Subnet      string   `json:"subnet"`
	Tags        []string `json:"tags"`
}

type InterfaceMetadata struct {
	DeviceId    string `json:"device_id"`
	Index       int32  `json:"index"` // IF-MIB ifIndex type is InterfaceIndex (Integer32 (1..2147483647))
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	MacAddress  string `json:"mac_address"`
	AdminStatus int32  `json:"admin_status"` // IF-MIB ifAdminStatus type is INTEGER
	OperStatus  int32  `json:"oper_status"`  // IF-MIB ifOperStatus type is INTEGER
}
