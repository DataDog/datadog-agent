package common

type LldpRemote struct {
	TimeMark         int
	LocalPortNum     int
	Index            int
	ChassisIdSubtype int
	ChassisId        string
	PortIdSubtype    int
	PortId           string
	PortDesc         string
	SysName          string
	SysDesc          string
	SysCapSupported  []byte
	SysCapEnabled    []byte
}

type LldpRemoteManagement struct {
	TimeMark         int
	LocalPortNum     int
	Index            int
	ManAddrSubtype   int
	ManAddr          string
	ManAddrIfSubtype int
}
