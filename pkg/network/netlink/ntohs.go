package netlink

import (
	"encoding/binary"
)

// NtohsU16 converts an unsigned int16 from network order to host order
func NtohsU16(n uint16) uint16 {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf[0:2], n)
	return binary.BigEndian.Uint16(buf)
}
