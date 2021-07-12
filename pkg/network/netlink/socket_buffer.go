// +build linux

package netlink

import (
	"syscall"

	"github.com/mdlayher/netlink"
)

// setSocketBufferSize of a netlink socket.
// We use this custom function as oposed to netlink.Conn.SetReadBuffer because you can only
// set a value higher than /proc/sys/net/core/rmem_default (which is around 200kb for most systems)
// if you use SO_RCVBUFFORCE with CAP_NET_ADMIN (https://linux.die.net/man/7/socket).
func setSocketBufferSize(sizeInBytes int, c *netlink.Conn) error {
	rawConn, err := c.SyscallConn()
	if err != nil {
		return err
	}

	var syscallErr error
	ctrlErr := rawConn.Control(func(fd uintptr) {
		syscallErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUFFORCE, sizeInBytes)
	})

	if ctrlErr != nil {
		return ctrlErr
	}
	if syscallErr != nil {
		return syscallErr
	}

	return nil
}

// getSocketBufferSize in bytes (rcvbuf)
func getSocketBufferSize(c *netlink.Conn) (int, error) {
	rawConn, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}

	var (
		syscallErr error
		bufferSize int
	)
	ctrlErr := rawConn.Control(func(fd uintptr) {
		bufferSize, syscallErr = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	})

	if ctrlErr != nil {
		return 0, ctrlErr
	}
	if syscallErr != nil {
		return 0, syscallErr
	}

	return bufferSize, nil
}
