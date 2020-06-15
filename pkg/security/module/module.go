package module

import (
	"os"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/module"
	aconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var RuleWithoutEventErr = errors.New("rule without event")

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
	m.probe.AddKernelFilter(event.(*sprobe.Event), field)
}

func (m *Module) EventApproverFound(event eval.Event, field string) {
	m.probe.AddKernelFilter(event.(*sprobe.Event), field)
}

func (m *Module) HandleEvent(event *sprobe.Event) {
	m.ruleSet.Evaluate(event)
}

func loadMacros(config *config.Config) (map[string]*ast.Macro, error) {
	macros := make(map[string]*ast.Macro)

	for _, policyDef := range config.Policies {
		for _, policyPath := range policyDef.Files {
			f, err := os.Open(policyPath)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load policy `%s`", policyPath)
			}

			policy, err := policy.LoadPolicy(f)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load policy `%s`", policyPath)
			}

			for _, macroDef := range policy.Macros {
				astMacro, err := ast.ParseMacro(macroDef.Expression)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to load policy `%s`", policyPath)
				}

				macros[macroDef.ID] = astMacro
			}
		}
	}

	return macros, nil
}

func LoadPolicies(config *config.Config, probe *sprobe.Probe) (*eval.RuleSet, error) {
	macros, err := loadMacros(config)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	opts := eval.Opts{
		Debug:     config.Debug,
		Macros:    macros,
		Constants: sprobe.SECLConstants,
	}

	ruleSet := probe.NewRuleSet(opts)

	for _, policyDef := range config.Policies {
		for _, policyPath := range policyDef.Files {
			log.Infof("loading security policies from `%s`", policyPath)
			f, err := os.Open(policyPath)
			if err != nil {
				log.Errorf("failed to load policy: %s", policyPath)
				return nil, err
			}

			policy, err := policy.LoadPolicy(f)
			if err != nil {
				log.Errorf("failed to load policy `%s`: %s", policyPath, err)
				return nil, err
			}

			for _, ruleDef := range policy.Rules {
				var tags []string
				for k, v := range ruleDef.Tags {
					tags = append(tags, k+":"+v)
				}

				astRule, err := ast.ParseRule(ruleDef.Expression)
				if err != nil {
					if err, ok := err.(*eval.AstToEvalError); ok {
						log.Errorf("rule syntax error: %s\n%s", err, secl.SprintExprAt(ruleDef.Expression, err.Pos))
					} else {
						log.Errorf("rule parsing error: %s\n%s", err, ruleDef.Expression)
					}
					return nil, err
				}

				rule, err := ruleSet.AddRule(ruleDef.ID, astRule, tags...)
				if err != nil {
					if err, ok := err.(*eval.AstToEvalError); ok {
						log.Errorf("rule syntax error: %s\n%s", err, secl.SprintExprAt(ruleDef.Expression, err.Pos))
					} else {
						log.Errorf("rule compilation error: %s\n%s", err, ruleDef.Expression)
					}
					return nil, err
				}

				if len(rule.GetEventTypes()) == 0 {
					log.Errorf("rule without event specified: %s", ruleDef.Expression)
					return nil, RuleWithoutEventErr
				}
			}
		}
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
