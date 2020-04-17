package agent

import (
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/fim"
)

type Agent struct {
}

func (a *Agent) Start() error {
	return probe.Manager.Start()
}

func NewAgent() *Agent {
	var monitors = []probe.EventMonitor{
		fim.Monitor,
	}

	probe.Manager = probe.NewProbeManager(probe.ProbeManagerOptions{}, monitors)

	return &Agent{}
}
