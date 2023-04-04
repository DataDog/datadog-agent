package types

// FourTuple represents a network four-tuple (source IP, destination IP, source port, destination port)
type FourTuple struct {
	SrcIPHigh uint64
	SrcIPLow  uint64

	DstIPHigh uint64
	DstIPLow  uint64

	// ports separated for alignment/size optimization
	SrcPort uint16
	DstPort uint16
}
