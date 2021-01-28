// +build !linux

package procutil

import (
	"fmt"
	"time"
)

// NewProcessProbe is currently not implemented in non-linux environments
func NewProcessProbe() *Probe {
	return nil
}

// Close is currently not implemented in non-linux environments
func (p *Probe) Close() {}

// StatsForPIDs is currently not implemented in non-linux environments
func (p *Probe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	return nil, fmt.Errorf("StatsForPIDs is not implemented in non-linux environment")
}

// ProcessesByPID is currently not implemented in non-linux environments
func (p *Probe) ProcessesByPID(now time.Time) (map[int32]*Process, error) {
	return nil, fmt.Errorf("ProcessesByPID is not implemented in non-linux environment")
}
