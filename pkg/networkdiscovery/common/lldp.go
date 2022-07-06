package common

type LldpRemote struct {
	TimeMark         int
	LocalPortNum     int
	Index            int
	ChassisIdSubtype int
	ChassisId        string
	PortIdSubType    int
	PortId           string
	PortDesc         string
	SysName          string
	SysDesc          string
	SysCapSupported  string // TODO: should be converted into flags/states
	SysCapEnabled    string // TODO: should be converted into flags/states
	RemoteManagement *LldpRemoteManagement
}

type LldpRemoteManagement struct {
	TimeMark         int
	LocalPortNum     int
	Index            int
	ManAddrSubtype   int
	ManAddr          string
	ManAddrIfSubtype int
}

var ChassisIdSubtypeMap = map[int]string{
	1: "chassisComponent",
	2: "interfaceAlias",
	3: "portComponent",
	4: "macAddress",
	5: "networkAddress",
	6: "interfaceName",
	7: "local",
}

var PortIdSubTypeMap = map[int]string{
	1: "interfaceAlias",
	2: "portComponent",
	3: "macAddress",
	4: "networkAddress",
	5: "interfaceName",
	6: "agentCircuitId",
	7: "local",
}
