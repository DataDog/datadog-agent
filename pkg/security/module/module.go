package module

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

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

func (a *Module) Register(server *grpc.Server) error {
	a.probe.SetEventHandler(a)

	eventServer := NewEventServer()
	api.RegisterSecurityModuleServer(server, eventServer)

	a.ruleSet.AddListener(eventServer)

	return a.probe.Start()
}

func (a *Module) Close() {
	a.probe.Stop()
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

func (a *Module) GetStats() map[string]interface{} {
	return nil
}

func NewModule(cfg *aconfig.AgentConfig) (module.Module, error) {
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

	return agent, nil
}
