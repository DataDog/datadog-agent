// +build !linux

package net

import "github.com/DataDog/datadog-agent/pkg/ebpf"

// RemoteNetTracerUtil is only implemented on linux
type RemoteNetTracerUtil struct{}

// SetNetworkTracerSocketPath is only implemented on linux
func SetNetworkTracerSocketPath(_ string) {
	// no-op
}

// GetRemoteNetworkTracerUtil is only implemented on linux
func GetRemoteNetworkTracerUtil() (*RemoteNetTracerUtil, error) {
	return &RemoteNetTracerUtil{}, nil
}

// GetConnections is only implemented on linux
func (r *RemoteNetTracerUtil) GetConnections(clientID string) ([]ebpf.ConnectionStats, error) {
	return nil, ebpf.ErrNotImplemented
}

// ShouldLogTracerUtilError is only implemented on linux
func ShouldLogTracerUtilError() bool {
	return false
}
