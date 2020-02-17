// +build !linux

package net

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// RemoteSysProbeUtil is only implemented on linux
type RemoteSysProbeUtil struct{}

// SetSystemProbeSocketPath is only implemented on linux
func SetSystemProbeSocketPath(_ string) {
	// no-op
}

// GetRemoteSystemProbeUtil is only implemented on linux
func GetRemoteSystemProbeUtil() (*RemoteSysProbeUtil, error) {
	return &RemoteSysProbeUtil{}, nil
}

// GetConnections is only implemented on linux
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// ShouldLogTracerUtilError is only implemented on linux
func ShouldLogTracerUtilError() bool {
	return false
}
