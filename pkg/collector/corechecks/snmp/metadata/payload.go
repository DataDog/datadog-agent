package metadata

// NetworkDevicesMetadata contains network devices metadata
type NetworkDevicesMetadata struct {
	Subnet     string              `json:"subnet"`
	Devices    []DeviceMetadata    `json:"devices"`
	Interfaces []InterfaceMetadata `json:"interfaces"`
}

// DeviceMetadata contains device metadata
type DeviceMetadata struct {
	ID          string   `json:"id"`
	IDTags      []string `json:"id_tags"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IPAddress   string   `json:"ip_address"`
	SysObjectID string   `json:"sys_object_id"`
	Profile     string   `json:"profile"`
	Vendor      string   `json:"vendor"`
	Subnet      string   `json:"subnet"`
	Tags        []string `json:"tags"`
}

// InterfaceMetadata contains interface metadata
type InterfaceMetadata struct {
	DeviceID    string `json:"device_id"`
	Index       int32  `json:"index"` // IF-MIB ifIndex type is InterfaceIndex (Integer32 (1..2147483647))
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	MacAddress  string `json:"mac_address"`
	AdminStatus int32  `json:"admin_status"` // IF-MIB ifAdminStatus type is INTEGER
	OperStatus  int32  `json:"oper_status"`  // IF-MIB ifOperStatus type is INTEGER
}
