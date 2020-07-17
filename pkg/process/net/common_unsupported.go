// +build !linux,!windows

package net

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// RemoteSysProbeUtil is not supported
type RemoteSysProbeUtil struct{}

// SetSystemProbePath is not supported
func SetSystemProbePath(_ string) {
	// no-op
}

// CheckPath is not supported
func CheckPath() error {
	return ebpf.ErrNotImplemented
}

// GetRemoteSystemProbeUtil is not supported
func GetRemoteSystemProbeUtil() (*RemoteSysProbeUtil, error) {
	return &RemoteSysProbeUtil{}, ebpf.ErrNotImplemented
}

// GetConnections is not supported
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// GetStats is not supported
func (r *RemoteSysProbeUtil) GetStats() (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}
