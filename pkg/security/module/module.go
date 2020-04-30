package module

import (
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/cmd/system-probe/module"
	aconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type Module struct {
	probe   *probe.Probe
	config  *config.Config
	ruleSet *eval.RuleSet
}

func (a *Module) Start() error {
	a.probe.SetEventHandler(a)
	a.ruleSet.AddListener(a)

	return a.probe.Start()
}

func (a *Module) Stop() {
	a.probe.Stop()
}

func (a *Module) RuleMatch(rule *eval.Rule, event eval.Event) {
	log.Printf("Event %+v matched against rule %+v", rule, event)
}

func (a *Module) DiscriminatorDiscovered(event eval.Event, field string) {
	a.probe.AddKernelFilter(event.(*probe.Event), field)
}

func (a *Module) HandleEvent(event *probe.Event) {
	a.ruleSet.Evaluate(event)
}

func (a *Module) LoadPolicies() error {
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

func (a *Module) GetStatus() module.Status {
	return module.Status{}
}

func NewModule(cfg *aconfig.AgentConfig, opts module.Opts) (module.Module, error) {
	config, err := config.NewConfig()
	if err != nil {
		return nil, errors.Wrap(err, "invalid security agent configuration")
	}

	probe, err := probe.NewProbe(config)
	if err != nil {
		return nil, err
	}

	agent := &Module{
		config:  config,
		probe:   probe,
		ruleSet: eval.NewRuleSet(probe.GetModel(), config.Debug),
	}

	if err := agent.LoadPolicies(); err != nil {
		return nil, err
	}

	server := NewEventServer()
	api.RegisterSecurityModuleServer(opts.GRPCServer, server)

	agent.ruleSet.AddListener(server)

	return agent, nil
}
