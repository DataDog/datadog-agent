package enrichment

import (
	"encoding/binary"
	"net"
)

// FormatMacAddress formats mac address from uint64 to "xx:xx:xx:xx:xx:xx" format
func FormatMacAddress(fieldValue uint64) string {
	mac := make([]byte, 8)
	binary.BigEndian.PutUint64(mac, fieldValue)
	return net.HardwareAddr(mac[2:]).String()
}
