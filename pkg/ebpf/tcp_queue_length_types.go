// +build linux

package ebpf

import (
	"net"
)

type QueueLength struct {
	Size int    `json:"size"`
	Min  uint32 `json:"min"`
	Max  uint32 `json:"max"`
}

type Conn struct {
	Saddr net.IP `json:"saddr"`
	Daddr net.IP `json:"daddr"`
	Sport uint16 `json:"sport"`
	Dport uint16 `json:"dport"`
}

type Stats struct {
	Pid         uint32      `json:"pid"`
	ContainerID string      `json:"containerid"`
	Conn        Conn        `json:"conn"`
	Rqueue      QueueLength `json:"read queue"`
	Wqueue      QueueLength `json:"write queue"`
}
