// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdhandoff

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// setPassCred enables credential passing from the kernel, which the Agent needs
// for DogStatsD origin detection. The option is a property of the socket, so it
// survives the handoff to the Agent.
func setPassCred(rawConn syscall.RawConn) error {
	var serr error
	if err := rawConn.Control(func(fd uintptr) {
		serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PASSCRED, 1)
	}); err != nil {
		return err
	}
	return serr
}
