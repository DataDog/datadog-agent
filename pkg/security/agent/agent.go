package agent

import (
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type Agent struct {
	probe   *probe.Probe
	config  *config.Config
	ruleSet *eval.RuleSet
}

func (a *Agent) Start() error {
	a.probe.SetEventHandler(a)
	a.ruleSet.AddListener(a)

	return a.probe.Start()
}

func (a *Agent) Stop() error {
	a.probe.Stop()
	return nil
}

func (a *Agent) RuleMatch(rule *eval.Rule, event eval.Event) {
	log.Printf("Event %+v matched against rule %+v", rule)
}

func (a *Agent) DiscriminatorDiscovered(event eval.Event, field string) {
	a.probe.AddKernelFilter(event.(*probe.Event), field)
}

func (a *Agent) HandleEvent(event *probe.Event) {
	a.ruleSet.Evaluate(event)
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

			for _, ruleDef := range policy.Rules {
				_, err := a.ruleSet.AddRule(ruleDef.Name, ruleDef.Expression)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func NewAgent() (*Agent, error) {
	config, err := config.NewConfig()
	if err != nil {
		return nil, errors.Wrap(err, "invalid security agent configuration")
	}

	probe, err := probe.NewProbe(config)
	if err != nil {
		return nil, err
	}

	agent := &Agent{
		config:  config,
		probe:   probe,
		ruleSet: eval.NewRuleSet(probe.GetModel(), config.Debug),
	}

	if err := agent.LoadPolicies(); err != nil {
		return nil, err
	}

	return agent, nil
}
