// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pidmapimpl implements a component for storing pid - containerID relations
package pidmapimpl

import (
	"errors"
	"sync"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type pidContainerMap map[int32]string

var (
	errPidMapUnavailable    = errors.New("no pid map has been set for this replay")
	errContainerUnavailable = errors.New("specified pid is not associated to any container")
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newPidMap),
	)
}

type state struct {
	mux    sync.RWMutex
	pidMap pidContainerMap
}

// SetPidMap sets the map with the pid - containerID relations
func (s *state) SetPidMap(m map[int32]string) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.pidMap = pidContainerMap{}
	for pid, containerID := range m {
		s.pidMap[pid] = containerID
	}
}

// ContainerIDForPID returns the matching container id for a pid, or an error if not found.
func (s *state) ContainerIDForPID(pid int32) (string, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	if s.pidMap == nil {
		return "", errPidMapUnavailable
	}

	cID, found := s.pidMap[pid]
	if !found {
		return "", errContainerUnavailable
	}

	return cID, nil

}

func newPidMap() pidmap.Component {
	return &state{}
}

// NewServerlessPidMap creates a new instance of pidmap.Component
func NewServerlessPidMap() pidmap.Component {
	return newPidMap()
}
