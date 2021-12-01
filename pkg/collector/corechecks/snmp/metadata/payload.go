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
	Namespace        string              `json:"namespace"`
	Devices          []DeviceMetadata    `json:"devices,omitempty"`
	Interfaces       []InterfaceMetadata `json:"interfaces,omitempty"`
	CollectTimestamp int64               `json:"collect_timestamp"`
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
}

// InterfaceMetadata contains interface metadata
type InterfaceMetadata struct {
	DeviceID    string   `json:"device_id"`
	IDTags      []string `json:"id_tags"` // used to correlate with interface metrics
	Index       int32    `json:"index"`   // IF-MIB ifIndex type is InterfaceIndex (Integer32 (1..2147483647))
	Name        string   `json:"name,omitempty"`
	Alias       string   `json:"alias,omitempty"`
	Description string   `json:"description,omitempty"`
	MacAddress  string   `json:"mac_address,omitempty"`
	AdminStatus int32    `json:"admin_status,omitempty"` // IF-MIB ifAdminStatus type is INTEGER
	OperStatus  int32    `json:"oper_status,omitempty"`  // IF-MIB ifOperStatus type is INTEGER
}
