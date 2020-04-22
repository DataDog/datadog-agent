package agent

import (
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/pkg/errors"
)

type Agent struct {
	probe           *probe.Probe
	config          *config.Config
	policies        []*policy.Policy
	rulesEvaluators []eval.Evaluator
}

func (a *Agent) Start() error {
	return a.probe.Start()
}

func (a *Agent) Stop() error {
	a.probe.Stop()
	return nil
}

func (a *Agent) TriggerSignal() {
}

func (a *Agent) HandleEvent(event interface{}) {
	context := &eval.Context{}

	for _, evaluator := range a.rulesEvaluators {
		if evaluator(context) {
			a.TriggerSignal()
		}
	}

	log.Printf("Handling event %s\n", reflect.TypeOf(event))
}

func (a *Agent) LoadPolicies() error {
	for _, policyDef := range a.config.Policies {
		for _, policyPath := range policyDef.Files {
			f, err := os.Open(policyPath)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to load policy '%s'", policyPath))
			}

			policy, err := policy.LoadPolicy(f)
			if err != nil {
				return err
			}

			a.policies = append(a.policies, policy)
		}
	}
	return nil
}

func NewAgent() (*Agent, error) {
	config, err := config.NewConfig()
	if err != nil {
		return nil, errors.Wrap(err, "invalid security agent configuration")
	}

	agent := &Agent{
		config: config,
	}

	agent.probe = probe.NewProbe(agent)

	if err := agent.LoadPolicies(); err != nil {
		return nil, err
	}

	return agent, nil
}
