package module

import (
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/module"
	aconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Module struct {
	probe   *sprobe.Probe
	config  *config.Config
	ruleSet *eval.RuleSet
	server  *EventServer
}

func (m *Module) Register(server *grpc.Server) error {
	if server != nil {
		api.RegisterSecurityModuleServer(server, m.server)
	}

	m.probe.SetEventHandler(m)
	m.ruleSet.AddListener(m)

	if err := m.probe.Start(); err != nil {
		return err
	}

	if err := m.probe.ApplyRuleSet(m.ruleSet); err != nil {
		log.Warnf(err.Error())
	}

	return nil
}

func (m *Module) Close() {
	m.probe.Stop()
}

func (m *Module) RuleMatch(rule *eval.Rule, event eval.Event) {
	m.server.SendEvent(rule, event)
}

func (m *Module) EventDiscarderFound(event eval.Event, field string) {
	m.probe.OnNewDiscarder(event.(*sprobe.Event), field)
}

func (m *Module) HandleEvent(event *sprobe.Event) {
	m.ruleSet.Evaluate(event)
}

func LoadPolicies(config *config.Config, probe *sprobe.Probe) (*eval.RuleSet, error) {
	var policySet policy.PolicySet
	// Load and parse policies
	for _, policyDef := range config.Policies {
		for _, policyPath := range policyDef.Files {
			// Open policy path
			f, err := os.Open(policyPath)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load policy `%s`", policyPath)
			}
			// Parse policy file
			policy, err := policy.LoadPolicy(f)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load policy `%s`", policyPath)
			}
			// Merge in the policy set
			policySet.AddPolicy(policy)
		}
	}
	// Create new ruleset with empty rules and macros
	ruleSet := probe.NewRuleSet(eval.NewOptsWithParams(config.Debug, sprobe.SECLConstants))
	// Add the macros to the ruleset and generate macros evaluators
	if err := ruleSet.AddMacros(policySet.Macros); err != nil {
		return nil, errors.Wrap(err, "couldn't add macros to the ruleset")
	}
	// Add rules to the ruleset and generate rules evaluators
	if err := ruleSet.AddRules(policySet.Rules); err != nil {
		return nil, errors.Wrap(err, "couldn't add rules to the ruleset")
	}
	return ruleSet, nil
}

func (m *Module) GetStats() map[string]interface{} {
	return nil
}

func (m *Module) GetRuleSet() *eval.RuleSet {
	return m.ruleSet
}

func NewModule(cfg *aconfig.AgentConfig) (module.Module, error) {
	config, err := config.NewConfig()
	if err != nil {
		log.Errorf("invalid security agent configuration: %s", err)
		return nil, err
	}

	probe, err := sprobe.NewProbe(config)
	if err != nil {
		return nil, err
	}

	ruleSet, err := LoadPolicies(config, probe)
	if err != nil {
		return nil, err
	}

	agent := &Module{
		config:  config,
		probe:   probe,
		ruleSet: ruleSet,
		server:  NewEventServer(),
	}

	return agent, nil
}
