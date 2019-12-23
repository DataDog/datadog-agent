// +build linux

package ebpf

import (
	"net"
)

/*
#include <stdint.h>
*/
import "C"

type QueueLength struct {
	Size C.int      `json:"size"`
	Min  C.uint32_t `json:"min"`
	Max  C.uint32_t `json:"max"`
}

type Stats struct {
	Pid    C.uint32_t  `json:"pid"`
	Rqueue QueueLength `json:"read queue"`
	Wqueue QueueLength `json:"write queue"`
}

type Conn struct {
	Saddr net.IP `json:"saddr"`
	Daddr net.IP `json:"daddr"`
	Sport uint16 `json:"sport"`
	Dport uint16 `json:"dport"`
}

type StatLine struct {
	Conn        Conn   `json:"conn"`
	ContainerID string `json:"containerid"`
	Stats       Stats  `json:"stats"`
}
