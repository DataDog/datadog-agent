package metadata

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

// NetworkDevicesMetadata contains network devices metadata
type NetworkDevicesMetadata struct {
	Subnet           string              `json:"subnet"`
	Devices          []DeviceMetadata    `json:"devices,omitempty"`
	Interfaces       []InterfaceMetadata `json:"interfaces,omitempty"`
	CollectTimestamp int64               `json:"collect_timestamp"`
}

// DeviceMetadata contains device metadata
type DeviceMetadata struct {
	ID          string       `json:"id"`
	IDTags      []string     `json:"id_tags"` // id_tags is the input to produce device.id, it's also used to correlated with device metrics.
	Name        string       `json:"name"`
	Description string       `json:"description"`
	IPAddress   string       `json:"ip_address"`
	SysObjectID string       `json:"sys_object_id"`
	Profile     string       `json:"profile"`
	Vendor      string       `json:"vendor"`
	Subnet      string       `json:"subnet"`
	Tags        []string     `json:"tags"`
	Status      DeviceStatus `json:"status"`
}

// InterfaceMetadata contains interface metadata
type InterfaceMetadata struct {
	DeviceID    string   `json:"device_id"`
	IDTags      []string `json:"id_tags"` // used to correlate with interface metrics
	Index       int32    `json:"index"`   // IF-MIB ifIndex type is InterfaceIndex (Integer32 (1..2147483647))
	Name        string   `json:"name"`
	Alias       string   `json:"alias"`
	Description string   `json:"description"`
	MacAddress  string   `json:"mac_address"`
	AdminStatus int32    `json:"admin_status"` // IF-MIB ifAdminStatus type is INTEGER
	OperStatus  int32    `json:"oper_status"`  // IF-MIB ifOperStatus type is INTEGER
}
