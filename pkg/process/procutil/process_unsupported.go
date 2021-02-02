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

// StatsForPIDsWithoutPerm is currently not implemented in non-linux environments
func (p *Probe) StatsForPIDsWithoutPerm(pids []int32, now time.Time) (map[int32]*Stats, error) {
	return nil, fmt.Errorf("StatsForPIDsWithoutPerm is not implemented in non-linux environment")
}

// StatsForPIDsWithPerm is currently not implemented in non-linux environments
func (p *Probe) StatsForPIDsWithPerm(pids []int32, now time.Time) (map[int32]*Stats, error) {
	return nil, fmt.Errorf("StatsForPIDsWithPerm is not implemented in non-linux environment")
}

// StatsForPIDs is currently not implemented in non-linux environments
func (p *Probe) StatsForPIDs(pids []int32, now time.Time, withPerm bool) (map[int32]*Stats, error) {
	return nil, fmt.Errorf("StatsForPIDs is not implemented in non-linux environment")
}

// ProcessesByPIDWithPerm is currently not implemented in non-linux environments
func (p *Probe) ProcessesByPIDWithPerm(now time.Time) (map[int32]*Process, error) {
	return nil, fmt.Errorf("ProcessesByPIDWithPerm is not implemented in non-linux environment")
}

// ProcessesByPIDWithoutPerm is currently not implemented in non-linux environments
func (p *Probe) ProcessesByPIDWithoutPerm(now time.Time) (map[int32]*Process, error) {
	return nil, fmt.Errorf("ProcessesByPIDWithoutPerm is not implemented in non-linux environment")
}

// ProcessesByPID is currently not implemented in non-linux environments
func (p *Probe) ProcessesByPID(now time.Time, withPerm bool) (map[int32]*Process, error) {
	return nil, fmt.Errorf("ProcessesByPID is not implemented in non-linux environment")
}

// StatsWithPermByPID is currently not implemented in non-linux environments
func (p *Probe) StatsWithPermByPID(returnZeroVals bool) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("StatsWithPermByPID is not implemented in non-linux environment")
}
