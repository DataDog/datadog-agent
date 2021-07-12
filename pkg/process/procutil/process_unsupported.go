// +build !linux

package procutil

import (
	"fmt"
	"time"
)

// Option is config options callback for system-probe
type Option func(p *Probe)

// WithReturnZeroPermStats configures whether StatsWithPermByPID() returns StatsWithPerm that
// has zero values on all fields
func WithReturnZeroPermStats(enabled bool) Option {
	return func(p *Probe) {
		p.returnZeroPermStats = enabled
	}
}

// WithPermission configures if process collection should fetch fields
// that require elevated permission or not
func WithPermission(enabled bool) Option {
	return func(p *Probe) {
		p.withPermission = enabled
	}
}

// NewProcessProbe returns a Probe object
func NewProcessProbe(options ...Option) *Probe {
	return &Probe{}
}

// Probe is an unimplemented struct for unsupported platforms
type Probe struct {
	returnZeroPermStats bool
	withPermission      bool
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

// StatsWithPermByPID is currently not implemented in non-linux environments
func (p *Probe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("StatsWithPermByPID is not implemented in non-linux environment")
}
