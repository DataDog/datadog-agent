package tcpqueuelength

import (
	"net"
)

// QueueLength contains the size and fullness extremums of a TCP Queue
type QueueLength struct {
	Size int    `json:"size"`
	Min  uint32 `json:"min"`
	Max  uint32 `json:"max"`
}

// Conn contains a TCP connection quadruplet
type Conn struct {
	Saddr net.IP `json:"saddr"`
	Daddr net.IP `json:"daddr"`
	Sport uint16 `json:"sport"`
	Dport uint16 `json:"dport"`
}

// Stats contains the statistics of a given socket
type Stats struct {
	Pid         uint32      `json:"pid"`
	ContainerID string      `json:"containerid"`
	Conn        Conn        `json:"conn"`
	Rqueue      QueueLength `json:"read queue"`
	Wqueue      QueueLength `json:"write queue"`
}
