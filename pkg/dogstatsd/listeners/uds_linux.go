// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"fmt"
	"net"

	log "github.com/cihub/seelog"
	"golang.org/x/sys/unix"
)

// getUDSAncillarySize gets the needed buffer size to retrieve the ancillary data
// from the out of band channel. We only get the header + 1 credentials struct
// and discard any information added by the sender.
func getUDSAncillarySize() int {
	return unix.CmsgSpace(unix.SizeofUcred) // Evaluates to 32 as of Go 1.8.3 on Linux 4.4.0
}

// enableUDSPassCred enables credential passing from the kernel for origin detection.
// That flag can be ignored if origin dection is disabled.
func enableUDSPassCred(conn *net.UnixConn) error {
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

// processUDSOrigin reads ancillary data to determine a packet's origin,
// it returns a string identifying the source.
// PID is added to ancillary data by the Linux kernel if we added the
// SO_PASSCRED to the socket, see enableUDSPassCred.
func processUDSOrigin(ancillary []byte) (string, error) {
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
	log.Debugf("dogstatsd-uds: packet from PID %d", cred.Pid)

	// FIXME: resolve PID to container name in another PR
	return fmt.Sprintf("pid:%d", cred.Pid), nil
}
