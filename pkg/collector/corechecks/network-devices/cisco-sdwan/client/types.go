// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

type viewKeys struct {
	UniqueKey     []string `json:"uniqueKey"`
	PreferenceKey string   `json:"preferenceKey"`
}

type fields struct {
	Property string `json:"property"`
	DataType string `json:"dataType"`
}

type columns struct {
	Title         string `json:"title"`
	Property      string `json:"property"`
	DataType      string `json:"dataType"`
	DisplayFormat string `json:"displayFormat,omitempty"`
	InputFormat   string `json:"inputFormat,omitempty"`
}

type chart struct {
}

type header struct {
	GeneratedOn int64     `json:"generatedOn"`
	ViewKeys    viewKeys  `json:"viewKeys"`
	Columns     []columns `json:"columns"`
	Fields      []fields  `json:"fields"`
	Chart       chart     `json:"chart"`
}

// PageInfo contains API pagination
type PageInfo struct {
	// Index based pagination
	StartID     string `json:"startId"`
	EndID       string `json:"endId"`
	MoreEntries bool   `json:"moreEntries"`
	Count       int    `json:"count"`

	// ScrollId based pagination
	ScrollID    string `json:"scrollId"`
	HasMoreData bool   `json:"hasMoreData"`
}

// Content is all supported data of this client
type Content interface {
	DeviceStatistics |
		Device |
		InterfaceStats |
		DeviceCounters |
		InterfaceState |
		CEdgeInterfaceState |
		AppRouteStatistics |
		ControlConnections |
		OMPPeer |
		BFDSession
}

// Response is a generic struct for API responses
type Response[T Content] struct {
	Header   header   `json:"header"`
	Data     []T      `json:"data"`
	PageInfo PageInfo `json:"pageInfo"`
}

// Device /dataservice/device
type Device struct {
	DeviceID            string   `json:"deviceId"`
	SystemIP            string   `json:"system-ip"`
	HostName            string   `json:"host-name"`
	Reachability        string   `json:"reachability"`
	Status              string   `json:"status"`
	Personality         string   `json:"personality"`
	DeviceType          string   `json:"device-type"`
	Timezone            string   `json:"timezone"`
	DeviceGroups        []string `json:"device-groups"`
	Lastupdated         float64  `json:"lastupdated"`
	DomainID            string   `json:"domain-id"`
	BoardSerial         string   `json:"board-serial"`
	CertificateValidity string   `json:"certificate-validity"`
	MaxControllers      string   `json:"max-controllers"`
	UUID                string   `json:"uuid"`
	ControlConnections  string   `json:"controlConnections"`
	DeviceModel         string   `json:"device-model"`
	Version             string   `json:"version"`
	ConnectedVManages   []string `json:"connectedVManages"`
	SiteID              string   `json:"site-id"`
	SiteName            string   `json:"site-name"`
	Latitude            string   `json:"latitude"`
	Longitude           string   `json:"longitude"`
	IsDeviceGeoData     bool     `json:"isDeviceGeoData"`
	Platform            string   `json:"platform"`
	UptimeDate          float64  `json:"uptime-date"`
	StatusOrder         float64  `json:"statusOrder"`
	DeviceOs            string   `json:"device-os"`
	Validity            string   `json:"validity"`
	State               string   `json:"state"`
	StateDescription    string   `json:"state_description"`
	ModelSku            string   `json:"model_sku"`
	LocalSystemIP       string   `json:"local-system-ip"`
	TotalCPUCount       string   `json:"total_cpu_count"`
	TestbedMode         bool     `json:"testbed_mode"`
	LayoutLevel         float64  `json:"layoutLevel"`
}

// InterfaceState /dataservice/data/device/state/interface
type InterfaceState struct {
	RecordID            string  `json:"recordId"`
	VdeviceName         string  `json:"vdevice-name"`
	IfAdminStatus       string  `json:"if-admin-status"`
	TCPMssAdjust        string  `json:"tcp-mss-adjust"`
	Duplex              string  `json:"duplex"`
	Rid                 float64 `json:"@rid"`
	Ifname              string  `json:"ifname"`
	AfType              string  `json:"af-type"`
	ShapingRate         float64 `json:"shaping-rate"`
	IfOperStatus        string  `json:"if-oper-status"`
	AutoNeg             string  `json:"auto-neg"`
	PauseType           string  `json:"pause-type"`
	Ipv6AdminStatus     string  `json:"ipv6-admin-status"`
	Ifindex             float64 `json:"ifindex"`
	AllowService        string  `json:"allow-service"`
	IfTrackerStatus     string  `json:"if-tracker-status"`
	CreateTimeStamp     float64 `json:"createTimeStamp"`
	SecondaryAddress    string  `json:"secondary-address"`
	VpnID               string  `json:"vpn-id"`
	VdeviceHostName     string  `json:"vdevice-host-name"`
	Mtu                 string  `json:"mtu"`
	Ipv6Address         string  `json:"ipv6-address"`
	Secondary           string  `json:"secondary"`
	IPAddress           string  `json:"ip-address"`
	Hwaddr              string  `json:"hwaddr"`
	SpeedMbps           string  `json:"speed-mbps"`
	VdeviceDataKey      string  `json:"vdevice-dataKey"`
	VmanageSystemIP     string  `json:"vmanage-system-ip"`
	Lastupdated         float64 `json:"lastupdated"`
	PortType            string  `json:"port-type"`
	BandwidthDownstream string  `json:"bandwidth-downstream"`
	BandwidthUpstream   string  `json:"bandwidth-upstream"`
	Desc                string  `json:"desc"`
	EncapType           string  `json:"encap-type"`
}

// DeviceStatistics /dataservice/data/device/statistics/devicesystemstatusstatistics
type DeviceStatistics struct {
	MemUsed         float64 `json:"mem_used"`
	DiskAvail       float64 `json:"disk_avail"`
	DeviceModel     string  `json:"device_model"`
	MemCached       float64 `json:"mem_cached"`
	MemUtil         float64 `json:"mem_util"`
	Min1Avg         float64 `json:"min1_avg"`
	DiskUsed        float64 `json:"disk_used"`
	Statcycletime   float64 `json:"statcycletime"`
	Tenant          string  `json:"tenant"`
	EntryTime       float64 `json:"entry_time"`
	Runningp        float64 `json:"runningp"`
	CPUUser         float64 `json:"cpu_user"`
	CPUIdleNew      float64 `json:"cpu_idle_new"`
	VipTime         float64 `json:"vip_time"`
	Min15Avg        float64 `json:"min15_avg"`
	Totalp          float64 `json:"totalp"`
	CPUIdle         float64 `json:"cpu_idle"`
	MemBuffers      float64 `json:"mem_buffers"`
	CPUSystem       float64 `json:"cpu_system"`
	VmanageSystemIP string  `json:"vmanage_system_ip"`
	Min5Avg         float64 `json:"min5_avg"`
	CPUMin1Avg      float64 `json:"cpu_min1_avg"`
	MemFree         float64 `json:"mem_free"`
	VdeviceName     string  `json:"vdevice_name"`
	VipIdx          float64 `json:"vip_idx"`
	CPUMin15Avg     float64 `json:"cpu_min15_avg"`
	SystemIP        string  `json:"system_ip"`
	CPUUserNew      float64 `json:"cpu_user_new"`
	CPUSystemNew    float64 `json:"cpu_system_new"`
	HostName        string  `json:"host_name"`
	CPUMin5Avg      float64 `json:"cpu_min5_avg"`
	ID              string  `json:"id"`
}

// CEdgeInterfaceState /dataservice/data/device/state/CEdgeInterface
type CEdgeInterfaceState struct {
	RecordID                string  `json:"recordId"`
	VdeviceName             string  `json:"vdevice-name"`
	RxErrors                float64 `json:"rx-errors"`
	IfAdminStatus           string  `json:"if-admin-status"`
	Ipv6TcpAdjustMss        string  `json:"ipv6-tcp-adjust-mss"`
	TxErrors                float64 `json:"tx-errors"`
	Rid                     float64 `json:"@rid"`
	Ifname                  string  `json:"ifname"`
	InterfaceType           string  `json:"interface-type"`
	IfOperStatus            string  `json:"if-oper-status"`
	Ifindex                 string  `json:"ifindex"`
	Ipv4TcpAdjustMss        string  `json:"ipv4-tcp-adjust-mss"`
	RxPackets               float64 `json:"rx-packets"`
	BiaAddress              string  `json:"bia-address"`
	CreateTimeStamp         float64 `json:"createTimeStamp"`
	VpnID                   string  `json:"vpn-id"`
	VdeviceHostName         string  `json:"vdevice-host-name"`
	Ipv4SubnetMask          string  `json:"ipv4-subnet-mask"`
	Mtu                     string  `json:"mtu"`
	TxDrops                 float64 `json:"tx-drops"`
	RxDrops                 float64 `json:"rx-drops"`
	IPAddress               string  `json:"ip-address"`
	IPV6Address             string  `json:"ipv6-addrs"`
	Hwaddr                  string  `json:"hwaddr"`
	SpeedMbps               string  `json:"speed-mbps"`
	AutoDownstreamBandwidth string  `json:"auto-downstream-bandwidth"`
	VdeviceDataKey          string  `json:"vdevice-dataKey"`
	VmanageSystemIP         string  `json:"vmanage-system-ip"`
	TxOctets                float64 `json:"tx-octets"`
	TxPackets               float64 `json:"tx-packets"`
	AutoUpstreamBandwidth   string  `json:"auto-upstream-bandwidth"`
	RxOctets                float64 `json:"rx-octets"`
	Lastupdated             float64 `json:"lastupdated"`
	Description             string  `json:"description"`
}

// InterfaceStats /dataservice/data/device/statistics/interfacestatistics
type InterfaceStats struct {
	DownCapacityPercentage float64 `json:"down_capacity_percentage"`
	TxPps                  float64 `json:"tx_pps"`
	TotalMbps              float64 `json:"total_mbps"`
	DeviceModel            string  `json:"device_model"`
	RxKbps                 float64 `json:"rx_kbps"`
	Interface              string  `json:"interface"`
	TxOctets               float64 `json:"tx_octets"`
	OperStatus             string  `json:"oper_status"`
	RxErrors               float64 `json:"rx_errors"`
	BwDown                 float64 `json:"bw_down"`
	TxPkts                 float64 `json:"tx_pkts"`
	TxErrors               float64 `json:"tx_errors"`
	RxOctets               float64 `json:"rx_octets"`
	Statcycletime          float64 `json:"statcycletime"`
	AdminStatus            string  `json:"admin_status"`
	BwUp                   float64 `json:"bw_up"`
	InterfaceType          string  `json:"interface_type"`
	Tenant                 string  `json:"tenant"`
	EntryTime              float64 `json:"entry_time"`
	VipTime                float64 `json:"vip_time"`
	AfType                 string  `json:"af_type"`
	RxPkts                 float64 `json:"rx_pkts"`
	RxPps                  float64 `json:"rx_pps"`
	VmanageSystemIP        string  `json:"vmanage_system_ip"`
	TxDrops                float64 `json:"tx_drops"`
	RxDrops                float64 `json:"rx_drops"`
	TxKbps                 float64 `json:"tx_kbps"`
	VdeviceName            string  `json:"vdevice_name"`
	UpCapacityPercentage   float64 `json:"up_capacity_percentage"`
	VipIdx                 float64 `json:"vip_idx"`
	HostName               string  `json:"host_name"`
	VpnID                  float64 `json:"vpn_id"`
	ID                     string  `json:"id"`
}

// DeviceCounters /dataservice/device/counters
type DeviceCounters struct {
	SystemIP                       string `json:"system-ip"`
	NumberVsmartControlConnections int    `json:"number-vsmart-control-connections"`
	ExpectedControlConnections     int    `json:"expectedControlConnections"`
	OmpPeersUp                     int    `json:"ompPeersUp"`
	OmpPeersDown                   int    `json:"ompPeersDown"`
	BfdSessionsUp                  int    `json:"bfdSessionsUp"`
	BfdSessionsDown                int    `json:"bfdSessionsDown"`
	IsMTEdge                       bool   `json:"isMTEdge"`
	RebootCount                    int    `json:"rebootCount"`
	CrashCount                     int    `json:"crashCount"`
}

// AppRouteStatistics /dataservice/data/device/statistics/approutestatsstatistics
type AppRouteStatistics struct {
	RemoteColor     string  `json:"remote_color"`
	FecRe           float64 `json:"fec_re"`
	VqoeScore       float64 `json:"vqoe_score"`
	DeviceModel     string  `json:"device_model"`
	Latency         float64 `json:"latency"`
	TxOctets        float64 `json:"tx_octets"`
	DstIP           string  `json:"dst_ip"`
	LocalColor      string  `json:"local_color"`
	SrcIP           string  `json:"src_ip"`
	SLAClassNames   string  `json:"sla_class_names"`
	Loss            float64 `json:"loss"`
	Total           float64 `json:"total"`
	TxPkts          float64 `json:"tx_pkts"`
	FecTx           float64 `json:"fec_tx"`
	RxOctets        float64 `json:"rx_octets"`
	Statcycletime   float64 `json:"statcycletime"`
	SiteID          float64 `json:"siteid"`
	State           string  `json:"state"`
	LocalSystemIP   string  `json:"local_system_ip"`
	Tenant          string  `json:"tenant"`
	EntryTime       float64 `json:"entry_time"`
	LossPercentage  float64 `json:"loss_percentage"`
	AppProbeClass   string  `json:"app_probe_class"`
	RxPkts          float64 `json:"rx_pkts"`
	VmanageSystemIP string  `json:"vmanage_system_ip"`
	FecRx           float64 `json:"fec_rx"`
	SrcPort         float64 `json:"src_port"`
	Jitter          float64 `json:"jitter"`
	RemoteSystemIP  string  `json:"remote_system_ip"`
	VdeviceName     string  `json:"vdevice_name"`
	Proto           string  `json:"proto"`
	VipIdx          float64 `json:"vip_idx"`
	DstPort         float64 `json:"dst_port"`
	Name            string  `json:"name"`
	SLAClassList    string  `json:"sla_class_list"`
	TunnelColor     string  `json:"tunnel_color"`
	HostName        string  `json:"host_name"`
	ID              string  `json:"id"`
}

// ControlConnections /dataservice/data/device/state/ControlConnection
type ControlConnections struct {
	RecordID          string  `json:"recordId"`
	Instance          float64 `json:"instance"`
	VdeviceName       string  `json:"vdevice-name"`
	SystemIP          string  `json:"system-ip"`
	RemoteColor       string  `json:"remote-color"`
	SiteID            float64 `json:"site-id"`
	ControllerGroupID float64 `json:"controller-group-id"`
	SharedRegionIDSet string  `json:"shared-region-id-set"`
	PeerType          string  `json:"peer-type"`
	Protocol          string  `json:"protocol"`
	Rid               float64 `json:"@rid"`
	State             string  `json:"state"`
	PrivateIP         string  `json:"private-ip"`
	DomainID          float64 `json:"domain-id"`
	BehindProxy       string  `json:"behind-proxy"`
	CreateTimeStamp   float64 `json:"createTimeStamp"`
	PrivatePort       float64 `json:"private-port"`
	VdeviceHostName   string  `json:"vdevice-host-name"`
	LocalColor        string  `json:"local-color"`
	VOrgName          string  `json:"v-org-name"`
	VdeviceDataKey    string  `json:"vdevice-dataKey"`
	VmanageSystemIP   string  `json:"vmanage-system-ip"`
	PublicIP          string  `json:"public-ip"`
	PublicPort        float64 `json:"public-port"`
	Lastupdated       float64 `json:"lastupdated"`
	UptimeDate        float64 `json:"uptime-date"`
}

// OMPPeer /dataservice/data/device/state/OMPPeer
type OMPPeer struct {
	RecordID        string  `json:"recordId"`
	DomainID        float64 `json:"domain-id"`
	VdeviceName     string  `json:"vdevice-name"`
	CreateTimeStamp float64 `json:"createTimeStamp"`
	Refresh         string  `json:"refresh"`
	SiteID          float64 `json:"site-id"`
	Type            string  `json:"type"`
	VdeviceHostName string  `json:"vdevice-host-name"`
	VdeviceDataKey  string  `json:"vdevice-dataKey"`
	Rid             float64 `json:"@rID"`
	VmanageSystemIP string  `json:"vmanage-system-ip"`
	Peer            string  `json:"peer"`
	Legit           string  `json:"legit"`
	Lastupdated     float64 `json:"lastupdated"`
	RegionID        string  `json:"region-id"`
	State           string  `json:"state"`
}

// BFDSession /dataservice/data/device/state/BFDSessions
type BFDSession struct {
	RecordID         string  `json:"recordId"`
	SrcIP            string  `json:"src-ip"`
	DstIP            string  `json:"dst-ip"`
	Color            string  `json:"color"`
	VdeviceName      string  `json:"vdevice-name"`
	SrcPort          float64 `json:"src-port"`
	CreateTimeStamp  float64 `json:"createTimeStamp"`
	SystemIP         string  `json:"system-ip"`
	DstPort          float64 `json:"dst-port"`
	SiteID           float64 `json:"site-id"`
	Transitions      float64 `json:"transitions"`
	VdeviceHostName  string  `json:"vdevice-host-name"`
	LocalColor       string  `json:"local-color"`
	DetectMultiplier string  `json:"detect-multiplier"`
	VdeviceDataKey   string  `json:"vdevice-dataKey"`
	Rid              float64 `json:"@rid"`
	VmanageSystemIP  string  `json:"vmanage-system-ip"`
	Proto            string  `json:"proto"`
	Lastupdated      float64 `json:"lastupdated"`
	TxInterval       float64 `json:"tx-interval"`
	State            string  `json:"state"`
	UptimeDate       float64 `json:"uptime-date"`
}
