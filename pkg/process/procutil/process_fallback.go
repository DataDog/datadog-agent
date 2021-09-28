// +build !linux,!windows

package procutil

import (
	"fmt"
	"time"

	"github.com/DataDog/gopsutil/process"
)

// NewProcessProbe returns a Probe object
func NewProcessProbe(options ...Option) Probe {
	p := &probe{}
	for _, option := range options {
		option(p)
	}
	return p
}

// probe is an implementation of the process probe for platforms other than Windows or Linux
type probe struct {
}

func (p *probe) Close() {}

func (p *probe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	procs, err := process.AllProcesses()
	if err != nil {
		return nil, err
	}
	return ConvertAllFilledProcessesToStats(procs), nil
}

func (p *probe) ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error) {
	procs, err := process.AllProcesses()
	if err != nil {
		return nil, err
	}
	return ConvertAllFilledProcesses(procs), nil
}

func (p *probe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("StatsWithPermByPID is not implemented in this environment")
}
