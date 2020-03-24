// +build linux

package net

import (
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

const (
	statusURL           = "http://unix/status"
	connectionsURL      = "http://unix/connections"
	contentTypeProtobuf = "application/protobuf"
	netType             = "unix"
)

// SetSystemProbePath provides a unix socket path location to be used by the remote system probe.
// This needs to be called before GetRemoteSystemProbeUtil.
func SetSystemProbePath(cfg *config.AgentConfig) {
	globalPath = cfg.SystemProbeSocketPath
}
