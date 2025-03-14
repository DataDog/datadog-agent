// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

type viewKeys struct {
	PreferenceKey string   `json:"preferenceKey"`
	UniqueKey     []string `json:"uniqueKey"`
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
	Chart       chart     `json:"chart"`
	ViewKeys    viewKeys  `json:"viewKeys"`
	Columns     []columns `json:"columns"`
	Fields      []fields  `json:"fields"`
	GeneratedOn int64     `json:"generatedOn"`
}

// PageInfo contains API pagination
type PageInfo struct {
	// Index based pagination
	StartID string `json:"startId"`
	EndID   string `json:"endId"`

	// ScrollId based pagination
	ScrollID string `json:"scrollId"`
	Count    int    `json:"count"`

	MoreEntries bool `json:"moreEntries"`
	HasMoreData bool `json:"hasMoreData"`
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
		BFDSession |
		HardwareEnvironment |
		CloudXStatistics |
		BGPNeighbor
}

// Response is a generic struct for API responses
type Response[T Content] struct {
	PageInfo PageInfo `json:"pageInfo"`
	Data     []T      `json:"data"`
	Header   header   `json:"header"`
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
	DomainID            string   `json:"domain-id"`
	BoardSerial         string   `json:"board-serial"`
	CertificateValidity string   `json:"certificate-validity"`
	MaxControllers      string   `json:"max-controllers"`
	UUID                string   `json:"uuid"`
	ControlConnections  string   `json:"controlConnections"`
	DeviceModel         string   `json:"device-model"`
	Version             string   `json:"version"`
	SiteID              string   `json:"site-id"`
	SiteName            string   `json:"site-name"`
	Latitude            string   `json:"latitude"`
	Longitude           string   `json:"longitude"`
	Platform            string   `json:"platform"`
	DeviceOs            string   `json:"device-os"`
	Validity            string   `json:"validity"`
	State               string   `json:"state"`
	StateDescription    string   `json:"state_description"`
	ModelSku            string   `json:"model_sku"`
	LocalSystemIP       string   `json:"local-system-ip"`
	TotalCPUCount       string   `json:"total_cpu_count"`
	DeviceGroups        []string `json:"device-groups"`
	ConnectedVManages   []string `json:"connectedVManages"`
	Lastupdated         float64  `json:"lastupdated"`
	UptimeDate          float64  `json:"uptime-date"`
	StatusOrder         float64  `json:"statusOrder"`
	LayoutLevel         float64  `json:"layoutLevel"`
	IsDeviceGeoData     bool     `json:"isDeviceGeoData"`
	TestbedMode         bool     `json:"testbed_mode"`
}

// InterfaceState /dataservice/data/device/state/interface
type InterfaceState struct {
	RecordID            string  `json:"recordId"`
	VdeviceName         string  `json:"vdevice-name"`
	IfAdminStatus       string  `json:"if-admin-status"`
	TCPMssAdjust        string  `json:"tcp-mss-adjust"`
	Duplex              string  `json:"duplex"`
	Ifname              string  `json:"ifname"`
	AfType              string  `json:"af-type"`
	IfOperStatus        string  `json:"if-oper-status"`
	AutoNeg             string  `json:"auto-neg"`
	PauseType           string  `json:"pause-type"`
	Ipv6AdminStatus     string  `json:"ipv6-admin-status"`
	AllowService        string  `json:"allow-service"`
	IfTrackerStatus     string  `json:"if-tracker-status"`
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
	PortType            string  `json:"port-type"`
	BandwidthDownstream string  `json:"bandwidth-downstream"`
	BandwidthUpstream   string  `json:"bandwidth-upstream"`
	Desc                string  `json:"desc"`
	EncapType           string  `json:"encap-type"`
	Rid                 float64 `json:"@rid"`
	ShapingRate         float64 `json:"shaping-rate"`
	Ifindex             float64 `json:"ifindex"`
	CreateTimeStamp     float64 `json:"createTimeStamp"`
	Lastupdated         float64 `json:"lastupdated"`
}

// DeviceStatistics /dataservice/data/device/statistics/devicesystemstatusstatistics
type DeviceStatistics struct {
	DeviceModel     string  `json:"device_model"`
	Tenant          string  `json:"tenant"`
	VmanageSystemIP string  `json:"vmanage_system_ip"`
	VdeviceName     string  `json:"vdevice_name"`
	SystemIP        string  `json:"system_ip"`
	HostName        string  `json:"host_name"`
	ID              string  `json:"id"`
	MemUsed         float64 `json:"mem_used"`
	DiskAvail       float64 `json:"disk_avail"`
	MemCached       float64 `json:"mem_cached"`
	MemUtil         float64 `json:"mem_util"`
	Min1Avg         float64 `json:"min1_avg"`
	DiskUsed        float64 `json:"disk_used"`
	Statcycletime   float64 `json:"statcycletime"`
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
	Min5Avg         float64 `json:"min5_avg"`
	CPUMin1Avg      float64 `json:"cpu_min1_avg"`
	MemFree         float64 `json:"mem_free"`
	VipIdx          float64 `json:"vip_idx"`
	CPUMin15Avg     float64 `json:"cpu_min15_avg"`
	CPUUserNew      float64 `json:"cpu_user_new"`
	CPUSystemNew    float64 `json:"cpu_system_new"`
	CPUMin5Avg      float64 `json:"cpu_min5_avg"`
}

// CEdgeInterfaceState /dataservice/data/device/state/CEdgeInterface
type CEdgeInterfaceState struct {
	RecordID                string  `json:"recordId"`
	VdeviceName             string  `json:"vdevice-name"`
	IfAdminStatus           string  `json:"if-admin-status"`
	Ipv6TcpAdjustMss        string  `json:"ipv6-tcp-adjust-mss"`
	Ifname                  string  `json:"ifname"`
	InterfaceType           string  `json:"interface-type"`
	IfOperStatus            string  `json:"if-oper-status"`
	Ifindex                 string  `json:"ifindex"`
	Ipv4TcpAdjustMss        string  `json:"ipv4-tcp-adjust-mss"`
	BiaAddress              string  `json:"bia-address"`
	VpnID                   string  `json:"vpn-id"`
	VdeviceHostName         string  `json:"vdevice-host-name"`
	Ipv4SubnetMask          string  `json:"ipv4-subnet-mask"`
	Mtu                     string  `json:"mtu"`
	IPAddress               string  `json:"ip-address"`
	IPV6Address             string  `json:"ipv6-addrs"`
	Hwaddr                  string  `json:"hwaddr"`
	SpeedMbps               string  `json:"speed-mbps"`
	AutoDownstreamBandwidth string  `json:"auto-downstream-bandwidth"`
	VdeviceDataKey          string  `json:"vdevice-dataKey"`
	VmanageSystemIP         string  `json:"vmanage-system-ip"`
	AutoUpstreamBandwidth   string  `json:"auto-upstream-bandwidth"`
	Description             string  `json:"description"`
	RxErrors                float64 `json:"rx-errors"`
	TxErrors                float64 `json:"tx-errors"`
	Rid                     float64 `json:"@rid"`
	RxPackets               float64 `json:"rx-packets"`
	CreateTimeStamp         float64 `json:"createTimeStamp"`
	TxDrops                 float64 `json:"tx-drops"`
	RxDrops                 float64 `json:"rx-drops"`
	TxOctets                float64 `json:"tx-octets"`
	TxPackets               float64 `json:"tx-packets"`
	RxOctets                float64 `json:"rx-octets"`
	Lastupdated             float64 `json:"lastupdated"`
}

// InterfaceStats /dataservice/data/device/statistics/interfacestatistics
type InterfaceStats struct {
	DeviceModel            string  `json:"device_model"`
	Interface              string  `json:"interface"`
	OperStatus             string  `json:"oper_status"`
	AdminStatus            string  `json:"admin_status"`
	InterfaceType          string  `json:"interface_type"`
	Tenant                 string  `json:"tenant"`
	AfType                 string  `json:"af_type"`
	VmanageSystemIP        string  `json:"vmanage_system_ip"`
	VdeviceName            string  `json:"vdevice_name"`
	HostName               string  `json:"host_name"`
	ID                     string  `json:"id"`
	DownCapacityPercentage float64 `json:"down_capacity_percentage"`
	TxPps                  float64 `json:"tx_pps"`
	TotalMbps              float64 `json:"total_mbps"`
	RxKbps                 float64 `json:"rx_kbps"`
	TxOctets               float64 `json:"tx_octets"`
	RxErrors               float64 `json:"rx_errors"`
	BwDown                 float64 `json:"bw_down"`
	TxPkts                 float64 `json:"tx_pkts"`
	TxErrors               float64 `json:"tx_errors"`
	RxOctets               float64 `json:"rx_octets"`
	Statcycletime          float64 `json:"statcycletime"`
	BwUp                   float64 `json:"bw_up"`
	EntryTime              float64 `json:"entry_time"`
	VipTime                float64 `json:"vip_time"`
	RxPkts                 float64 `json:"rx_pkts"`
	RxPps                  float64 `json:"rx_pps"`
	TxDrops                float64 `json:"tx_drops"`
	RxDrops                float64 `json:"rx_drops"`
	TxKbps                 float64 `json:"tx_kbps"`
	UpCapacityPercentage   float64 `json:"up_capacity_percentage"`
	VipIdx                 float64 `json:"vip_idx"`
	VpnID                  float64 `json:"vpn_id"`
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
	DeviceModel     string  `json:"device_model"`
	DstIP           string  `json:"dst_ip"`
	LocalColor      string  `json:"local_color"`
	SrcIP           string  `json:"src_ip"`
	SLAClassNames   string  `json:"sla_class_names"`
	State           string  `json:"state"`
	LocalSystemIP   string  `json:"local_system_ip"`
	Tenant          string  `json:"tenant"`
	AppProbeClass   string  `json:"app_probe_class"`
	VmanageSystemIP string  `json:"vmanage_system_ip"`
	RemoteSystemIP  string  `json:"remote_system_ip"`
	VdeviceName     string  `json:"vdevice_name"`
	Proto           string  `json:"proto"`
	Name            string  `json:"name"`
	SLAClassList    string  `json:"sla_class_list"`
	TunnelColor     string  `json:"tunnel_color"`
	HostName        string  `json:"host_name"`
	ID              string  `json:"id"`
	FecRe           float64 `json:"fec_re"`
	VqoeScore       float64 `json:"vqoe_score"`
	Latency         float64 `json:"latency"`
	TxOctets        float64 `json:"tx_octets"`
	Loss            float64 `json:"loss"`
	Total           float64 `json:"total"`
	TxPkts          float64 `json:"tx_pkts"`
	FecTx           float64 `json:"fec_tx"`
	RxOctets        float64 `json:"rx_octets"`
	Statcycletime   float64 `json:"statcycletime"`
	SiteID          float64 `json:"siteid"`
	EntryTime       float64 `json:"entry_time"`
	LossPercentage  float64 `json:"loss_percentage"`
	RxPkts          float64 `json:"rx_pkts"`
	FecRx           float64 `json:"fec_rx"`
	SrcPort         float64 `json:"src_port"`
	Jitter          float64 `json:"jitter"`
	VipIdx          float64 `json:"vip_idx"`
	DstPort         float64 `json:"dst_port"`
}

// ControlConnections /dataservice/data/device/state/ControlConnection
type ControlConnections struct {
	RecordID          string  `json:"recordId"`
	VdeviceName       string  `json:"vdevice-name"`
	SystemIP          string  `json:"system-ip"`
	RemoteColor       string  `json:"remote-color"`
	SharedRegionIDSet string  `json:"shared-region-id-set"`
	PeerType          string  `json:"peer-type"`
	Protocol          string  `json:"protocol"`
	State             string  `json:"state"`
	PrivateIP         string  `json:"private-ip"`
	BehindProxy       string  `json:"behind-proxy"`
	VdeviceHostName   string  `json:"vdevice-host-name"`
	LocalColor        string  `json:"local-color"`
	VOrgName          string  `json:"v-org-name"`
	VdeviceDataKey    string  `json:"vdevice-dataKey"`
	VmanageSystemIP   string  `json:"vmanage-system-ip"`
	PublicIP          string  `json:"public-ip"`
	Instance          float64 `json:"instance"`
	SiteID            float64 `json:"site-id"`
	ControllerGroupID float64 `json:"controller-group-id"`
	Rid               float64 `json:"@rid"`
	DomainID          float64 `json:"domain-id"`
	CreateTimeStamp   float64 `json:"createTimeStamp"`
	PrivatePort       float64 `json:"private-port"`
	PublicPort        float64 `json:"public-port"`
	Lastupdated       float64 `json:"lastupdated"`
	UptimeDate        float64 `json:"uptime-date"`
}

// OMPPeer /dataservice/data/device/state/OMPPeer
type OMPPeer struct {
	RecordID        string  `json:"recordId"`
	VdeviceName     string  `json:"vdevice-name"`
	Refresh         string  `json:"refresh"`
	Type            string  `json:"type"`
	VdeviceHostName string  `json:"vdevice-host-name"`
	VdeviceDataKey  string  `json:"vdevice-dataKey"`
	VmanageSystemIP string  `json:"vmanage-system-ip"`
	Peer            string  `json:"peer"`
	Legit           string  `json:"legit"`
	RegionID        string  `json:"region-id"`
	State           string  `json:"state"`
	DomainID        float64 `json:"domain-id"`
	CreateTimeStamp float64 `json:"createTimeStamp"`
	SiteID          float64 `json:"site-id"`
	Rid             float64 `json:"@rID"`
	Lastupdated     float64 `json:"lastupdated"`
}

// BFDSession /dataservice/data/device/state/BFDSessions
type BFDSession struct {
	RecordID         string  `json:"recordId"`
	SrcIP            string  `json:"src-ip"`
	DstIP            string  `json:"dst-ip"`
	Color            string  `json:"color"`
	VdeviceName      string  `json:"vdevice-name"`
	SystemIP         string  `json:"system-ip"`
	VdeviceHostName  string  `json:"vdevice-host-name"`
	LocalColor       string  `json:"local-color"`
	DetectMultiplier string  `json:"detect-multiplier"`
	VdeviceDataKey   string  `json:"vdevice-dataKey"`
	VmanageSystemIP  string  `json:"vmanage-system-ip"`
	Proto            string  `json:"proto"`
	State            string  `json:"state"`
	SrcPort          float64 `json:"src-port"`
	CreateTimeStamp  float64 `json:"createTimeStamp"`
	DstPort          float64 `json:"dst-port"`
	SiteID           float64 `json:"site-id"`
	Transitions      float64 `json:"transitions"`
	Rid              float64 `json:"@rid"`
	Lastupdated      float64 `json:"lastupdated"`
	TxInterval       float64 `json:"tx-interval"`
	UptimeDate       float64 `json:"uptime-date"`
}

// HardwareEnvironment /dataservice/data/device/state/HardwareEnvironment
type HardwareEnvironment struct {
	RecordID        string `json:"recordId"`
	VdeviceName     string `json:"vdevice-name"`
	VdeviceHostName string `json:"vdevice-host-name"`
	Measurement     string `json:"measurement"`
	VdeviceDataKey  string `json:"vdevice-dataKey"`
	VmanageSystemIP string `json:"vmanage-system-ip"`
	HwItem          string `json:"hw-item"`
	HwClass         string `json:"hw-class"`
	Status          string `json:"status"`
	HwDevIndex      int    `json:"hw-dev-index"`
	CreateTimeStamp int64  `json:"createTimeStamp"`
	Rid             int    `json:"@rid"`
	Lastupdated     int64  `json:"lastupdated"`
}

// CloudXStatistics /dataservice/data/device/statistics/cloudxstatistics
type CloudXStatistics struct {
	RemoteColor      string  `json:"remote_color"`
	DeviceModel      string  `json:"device_model"`
	Interface        string  `json:"interface"`
	LocalColor       string  `json:"local_color"`
	GatewaySystemIP  string  `json:"gateway_system_ip"`
	SourcePublicIP   string  `json:"source_public_ip"`
	LocalSystemIP    string  `json:"local_system_ip"`
	Tenant           string  `json:"tenant"`
	VqeStatus        string  `json:"vqe_status"`
	ExitType         string  `json:"exit_type"`
	VmanageSystemIP  string  `json:"vmanage_system_ip"`
	NbarAppGroupName string  `json:"nbar_app_group_name"`
	Application      string  `json:"application"`
	VdeviceName      string  `json:"vdevice_name"`
	BestPath         string  `json:"best_path"`
	VqeScore         string  `json:"vqe_score"`
	ServiceArea      string  `json:"service_area"`
	HostName         string  `json:"host_name"`
	AppURLHostIP     string  `json:"app_url_host_ip"`
	ID               string  `json:"id"`
	Latency          float64 `json:"latency"`
	Loss             float64 `json:"loss"`
	Statcycletime    float64 `json:"statcycletime"`
	EntryTime        float64 `json:"entry_time"`
	VipTime          float64 `json:"vip_time"`
	VipIdx           float64 `json:"vip_idx"`
	SiteID           float64 `json:"site_id"`
	VpnID            float64 `json:"vpn_id"`
}

// BGPNeighbor /dataservice/data/device/state/BGPNeighbor
type BGPNeighbor struct {
	RecordID        string  `json:"recordId"`
	VdeviceName     string  `json:"vdevice-name"`
	Afi             string  `json:"afi"`
	VdeviceHostName string  `json:"vdevice-host-name"`
	PeerAddr        string  `json:"peer-addr"`
	VdeviceDataKey  string  `json:"vdevice-dataKey"`
	VmanageSystemIP string  `json:"vmanage-system-ip"`
	State           string  `json:"state"`
	CreateTimeStamp float64 `json:"createTimeStamp"`
	VpnID           float64 `json:"vpn-id"`
	AS              float64 `json:"as"`
	Rid             float64 `json:"@rid"`
	AfiID           float64 `json:"afi-id"`
	Lastupdated     float64 `json:"lastupdated"`
}
