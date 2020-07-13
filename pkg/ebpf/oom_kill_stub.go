// +build !linux_bpf linux_bpf,!bcc

package ebpf

import "github.com/DataDog/datadog-agent/pkg/ebpf/oomkill"

// OOMKillProbe is not implemented on non-linux systems
type OOMKillProbe struct{}

// NewOOMKillProbe is not implemented on non-linux systems
func NewOOMKillProbe() (*OOMKillProbe, error) {
	return nil, ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *OOMKillProbe) Close() {}

// Get is not implemented on non-linux systems
func (t *OOMKillProbe) Get() []oomkill.Stats {
	return nil
}

// GetAndFlush is not implemented on non-linux systems
func (t *OOMKillProbe) GetAndFlush() []oomkill.Stats {
	return nil
}
