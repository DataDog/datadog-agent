package listeners

import (
	"fmt"
	"net"

	log "github.com/cihub/seelog"
	"golang.org/x/sys/unix"
)

const oob_size = 32 // FIXME confirm that

// enablePassCred enables credential passing from the kernel for origin detection.
// That flag can be ignored if origin dection is disabled.
func enablePassCred(conn *net.UnixConn) error {
	f, err := conn.File()
	defer f.Close()

	if err != nil {
		return err
	}
	fd := int(f.Fd())
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_PASSCRED, 1)
	if err != nil {
		return err
	}
	return nil
}

// processOrigin reads ancillary data to determine a packet's origin.
// It returns a string identifying the source
func processOrigin(ancillary []byte) (string, error) {
	messages, err := unix.ParseSocketControlMessage(ancillary)
	if err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("ancillary data empty")
	}
	cred, err := unix.ParseUnixCredentials(&messages[0])
	if err != nil {
		return "", err
	}
	log.Debugf("dogstatsd: packet from PID %d", cred.Pid)

	// FIXME: resolve PID to container name in another PR
	return fmt.Sprintf("pid:%d", cred.Pid), nil
}
