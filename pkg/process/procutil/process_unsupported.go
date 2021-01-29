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

// Probe is an empty struct for unsupported platforms
type Probe struct{}

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

// StatsWithPermByPID is currently not implemented in non-linux environments
func (p *Probe) StatsWithPermByPID(returnZeroVals bool) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("StatsWithPermByPID is not implemented in non-linux environment")
}
