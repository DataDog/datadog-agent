// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
)

const (
	pidToEntityCacheKeyPrefix = "pid_to_entity"
	pidToEntityCacheDuration  = time.Minute
)

// ErrNoContainerMatch is returned when no container ID can be matched
var errNoContainerMatch = errors.New("cannot match a container ID")

// getUDSAncillarySize gets the needed buffer size to retrieve the ancillary data
// from the out of band channel. We only get the header + 1 credentials struct
// and discard any information added by the sender.
func getUDSAncillarySize() int {
	return unix.CmsgSpace(unix.SizeofUcred) // Evaluates to 32 as of Go 1.8.3 on Linux 4.4.0
}

// enableUDSPassCred enables credential passing from the kernel for origin detection.
// That flag can be ignored if origin dection is disabled.
func enableUDSPassCred(conn *net.UnixConn) error {
	rawconn, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	return rawconn.Control(func(fd uintptr) {
		unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PASSCRED, 1)
	})
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

	if cred.Pid == 0 {
		return NoOrigin, fmt.Errorf("matched PID for the process is 0, it belongs " +
			"probably to another namespace. Is the agent in host PID mode?")
	}

	entity, err := getEntityForPID(cred.Pid)
	if err != nil {
		return NoOrigin, err
	}
	return entity, nil
}

// getEntityForPID returns the container entity name and caches the value for future lookups
// As the result is cached and the lookup is really fast (parsing local files), it can be
// called from the intake goroutine.
func getEntityForPID(pid int32) (string, error) {
	key := cache.BuildAgentKey(pidToEntityCacheKeyPrefix, strconv.Itoa(int(pid)))
	if x, found := cache.Cache.Get(key); found {
		return x.(string), nil
	}

	entity, err := entityForPID(pid)
	switch err {
	case nil:
		// No error, yay!
		cache.Cache.Set(key, entity, pidToEntityCacheDuration)
		return entity, nil
	case errNoContainerMatch:
		// No runtime detected, cache the `NoOrigin` result
		cache.Cache.Set(key, NoOrigin, pidToEntityCacheDuration)
		return NoOrigin, nil
	default:
		// Other lookup error, retry next time
		return NoOrigin, err
	}
}

// entityForPID returns the entity ID for a given PID. It can return
// errNoContainerMatch if no match is found for the PID.
func entityForPID(pid int32) (string, error) {
	cID, err := providers.ContainerImpl.ContainerIDForPID(int(pid))
	if err != nil {
		return "", err
	}
	if cID == "" {
		return "", errNoContainerMatch
	}

	return containers.BuildTaggerEntityName(cID), nil
}
