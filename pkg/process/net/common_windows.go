// +build windows

package net

import (
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

const (
	statusURL           = "http://localhost:3333/status"
	connectionsURL      = "http://localhost:3333/connections"
	contentTypeProtobuf = "application/protobuf"
	netType             = "tcp"
)

// SetSystemProbePath provides a unix socket path location to be used by the remote system probe.
// This needs to be called before GetRemoteSystemProbeUtil.
func SetSystemProbePath(cfg *config.AgentConfig) {
	globalPath = cfg.WinProbePath
}

