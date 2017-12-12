// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"fmt"
	"net"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"golang.org/x/sys/unix"
)

const (
	// PIDToContainerKeyPrefix holds the name prefix for cache keys
	PIDToContainerKeyPrefix = "pid_to_container"
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
		return NoOrigin, err
	}
	if len(messages) == 0 {
		return NoOrigin, fmt.Errorf("ancillary data empty")
	}
	cred, err := unix.ParseUnixCredentials(&messages[0])
	if err != nil {
		return NoOrigin, err
	}
	container, err := getContainerForPID(cred.Pid)
	if err != nil {
		return NoOrigin, err
	}
	return container, nil
}

// getContainerForPID returns the docker container id and caches the value for future lookups
// As the result is cached and the lookup is really fast (parsing a local file), it can be
// called from the intake goroutine.
func getContainerForPID(pid int32) (string, error) {
	key := cache.BuildAgentKey(PIDToContainerKeyPrefix, strconv.Itoa(int(pid)))
	if x, found := cache.Cache.Get(key); found {
		return x.(string), nil
	}
	id, err := docker.ContainerIDForPID(int(pid))
	if err != nil {
		return NoOrigin, err
	}

	var value string
	if len(id) == 0 {
		// If no container is found, it's probably a host process,
		// cache the `NoOrigin` result for future packets
		value = NoOrigin
	} else {
		value = fmt.Sprintf("docker://%s", id)
	}

	cache.Cache.Set(key, value, 0)
	return value, err
}
