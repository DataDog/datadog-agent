package agent

import (
	"log"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

type Agent struct {
	probe *probe.Probe
}

func (a *Agent) Start() error {
	return a.probe.Start()
}

func (a *Agent) Stop() error {
	a.probe.Stop()
	return nil
}

func (a *Agent) HandleEvent(event interface{}) {
	log.Printf("Handling event %s\n", reflect.TypeOf(event))
}

func NewAgent() *Agent {
	agent := &Agent{}
	agent.probe = probe.NewProbe(agent)
	return agent
}
