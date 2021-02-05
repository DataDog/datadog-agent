// +build !linux

package procutil

// Probe for non-linux system is currently not supported
type Probe struct{}

// NewProcessProbe would create an empty Probe object for compatibility
func NewProcessProbe() *Probe { return &Probe{} }
