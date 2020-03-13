// +build !linux,!windows

package net

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
)

// RemoteSysProbeUtil is not supported
type RemoteSysProbeUtil struct{}

// SetSystemProbeSocketPath is not supported
func SetSystemProbeSocketPath(_ string) {
	// no-op
}

// GetRemoteSystemProbeUtil is not supported
func GetRemoteSystemProbeUtil() (*RemoteSysProbeUtil, error) {
	return &RemoteSysProbeUtil{}, nil
}

// GetConnections is not supported
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	return nil, network.ErrNotImplemented
}

// ShouldLogTracerUtilError is not supported
func ShouldLogTracerUtilError() bool {
	return false
}
