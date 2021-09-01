//+build linux

package ebpf

func (c ConnType) String() string {
	if c == TCP {
		return "TCP"
	}
	return "UDP"
}

func (c ConnFamily) String() string {
	if c == IPv4 {
		return "v4"
	}
	return "v6"
}
