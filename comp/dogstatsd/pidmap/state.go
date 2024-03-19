package pidmap

import (
	"errors"
	"sync"
)

type pidContainerMap map[int32]string

var (
	errPidMapUnavailable    = errors.New("no pid map has been set for this replay")
	errContainerUnavailable = errors.New("specified pid is not associated to any container")
)

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

func newPidMap() Component {
	return &state{}
}
