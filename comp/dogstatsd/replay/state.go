// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"errors"
	"sync"
)

type pidContainerMap map[int32]string

var (
	mux    sync.RWMutex
	pidMap pidContainerMap

	errPidMapUnavailable    = errors.New("no pid map has been set for this replay")
	errContainerUnavailable = errors.New("specified pid is not associated to any container")
)

// SetPidMap sets the map with the pid - containerID relations
func SetPidMap(m map[int32]string) {
	panic("not called")
}

// ContainerIDForPID returns the matching container id for a pid, or an error if not found.
func ContainerIDForPID(pid int32) (string, error) {
	panic("not called")
}
