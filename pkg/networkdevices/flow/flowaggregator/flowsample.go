package flowaggregator

// DeviceFlowSample contains a device flow sample
type DeviceFlowSample struct {
	SrcAddr         string
	DstAddr         string
	SamplerAddr     string
	FlowType        string
	Proto           uint32
	InputInterface  uint32
	OutputInterface uint32
	Direction       uint32
	Bytes           uint64
	Packets         uint64
}
