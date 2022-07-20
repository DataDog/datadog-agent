package common

// LldpRemote TODO
type LldpRemote struct {
	TimeMark         int
	LocalPortNum     int
	Index            int
	ChassisIDSubtype int
	ChassisID        string
	PortIDSubType    int
	PortID           string
	PortDesc         string
	SysName          string
	SysDesc          string
	SysCapSupported  string // TODO: should be converted into flags/states
	SysCapEnabled    string // TODO: should be converted into flags/states
	RemoteManagement *LldpRemoteManagement
	LocalPort        *LldpLocPort
}

// LldpRemoteManagement TODO
type LldpRemoteManagement struct {
	TimeMark         int
	LocalPortNum     int
	Index            int
	ManAddrSubtype   int
	ManAddr          string
	ManAddrIfSubtype int
}

// LldpLocPort TODO
type LldpLocPort struct {
	PortNum       int
	PortIDSubType int
	PortID        string
	PortDesc      string
}

// ChassisIDSubtypeMap TODO
var ChassisIDSubtypeMap = map[int]string{
	1: "chassisComponent",
	2: "interfaceAlias",
	3: "portComponent",
	4: "macAddress",
	5: "networkAddress",
	6: "interfaceName",
	7: "local",
}

// PortIDSubTypeMap TODO
var PortIDSubTypeMap = map[int]string{
	1: "interfaceAlias",
	2: "portComponent",
	3: "macAddress",
	4: "networkAddress",
	5: "interfaceName",
	6: "agentCircuitId",
	7: "local",
}

// RemManAddrSubtype TODO
var RemManAddrSubtype = map[int]string{
	0:     "other",
	1:     "ipV4",
	2:     "ipV6",
	3:     "nsap",
	4:     "hdlc",
	5:     "bbn1822",
	6:     "all802",
	7:     "e163",
	8:     "e164",
	9:     "f69",
	10:    "x121",
	11:    "ipx",
	12:    "appletalk",
	13:    "decnetIV",
	14:    "banyanVines",
	15:    "e164withNsap",
	16:    "dns",
	17:    "distinguishedname",
	18:    "asnumber",
	19:    "xtpoveripv4",
	20:    "xtpoveripv6",
	21:    "xtpnativemodextp",
	65535: "reserved",
}
