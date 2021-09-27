// +build !linux,!windows

package procutil

import (
	"fmt"
	"time"
)

// NewProcessProbe returns a Probe object
func NewProcessProbe(options ...Option) Probe {
	p := &probe{}
	for _, option := range options {
		option(p)
	}
	return p
}

// probe is an unimplemented struct for unsupported platforms
type probe struct {
}

func (p *probe) Close() {}

func (p *probe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	return nil, fmt.Errorf("StatsForPIDs is not implemented in this environment")
}

func (p *probe) ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error) {
	return nil, fmt.Errorf("ProcessesByPID is not implemented in this environment")
}

func (p *probe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("StatsWithPermByPID is not implemented in this environment")
}
