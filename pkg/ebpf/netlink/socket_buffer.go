// +build linux

package netlink

import (
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/mdlayher/netlink"
)

// setSocketBufferSize of a netlink socket.
// We use this custom function as oposed to netlink.Conn.SetReadBuffer because you can only
// set a value higher than /proc/sys/net/core/rmem_default (which is around 200kb for most systems)
// if you use SO_RCVBUFFORCE with CAP_NET_ADMIN (https://linux.die.net/man/7/socket).
func setSocketBufferSize(sizeInBytes int, c *netlink.Conn) {
	rawConn, err := c.SyscallConn()
	if err != nil {
		log.Warnf("error obtaining raw_conn %s", err)
		return
	}

	ctrlErr := rawConn.Control(func(fd uintptr) {
		err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUFFORCE, sizeInBytes)
		if err != nil {
			log.Warnf("error executing setsockopt: %s", err)
		}
	})

	if ctrlErr != nil {
		log.Warnf("error executing executing socket operation: %s", ctrlErr)
	}

	ctrlErr = rawConn.Control(func(fd uintptr) {
		value, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
		if err != nil {
			log.Warnf("error executing getsockopt: %s", err)
			return
		}

		log.Infof("netlink socket rcvbuf size is %d bytes", value)
	})

	if ctrlErr != nil {
		log.Warnf("error executing executing socket operation: %s", ctrlErr)
	}
}
