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
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var RuleWithoutEventErr = errors.New("rule without event")

type Module struct {
	probe   *probe.Probe
	config  *config.Config
	ruleSet *eval.RuleSet
	server  *EventServer
}

func (a *Module) Register(server *grpc.Server) error {
	a.probe.SetEventHandler(a)
	api.RegisterSecurityModuleServer(server, a.server)

	a.ruleSet.AddListener(a)

	if err := a.probe.Start(); err != nil {
		return err
	}

	if err := a.probe.SetEventTypes(a.ruleSet.GetEventTypes()); err != nil {
		log.Warnf(err.Error())
	}

	return nil
}

func (a *Module) Close() {
	a.probe.Stop()
}

func (a *Module) RuleMatch(rule *eval.Rule, event eval.Event) {
	a.server.SendEvent(rule, event)
}

func (a *Module) EventDiscarderFound(event eval.Event, field string) {
	a.probe.AddKernelFilter(event.(*probe.Event), field)
}

func (a *Module) EventApproverFound(event eval.Event, field string) {
	a.probe.AddKernelFilter(event.(*probe.Event), field)
}

func (a *Module) HandleEvent(event *probe.Event) {
	a.ruleSet.Evaluate(event)
}

func (a *Module) loadMacros() (map[string]*ast.Macro, error) {
	macros := make(map[string]*ast.Macro)

	for _, policyDef := range a.config.Policies {
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

func (a *Module) LoadPolicies(model eval.Model, debug bool) error {
	macros, err := a.loadMacros()
	if err != nil {
		log.Error(err)
		return err
	}

	opts := eval.Opts{
		Debug:     debug,
		Macros:    macros,
		Constants: probe.SECLConstants,
	}

	a.ruleSet = eval.NewRuleSet(model, opts)

	for _, policyDef := range a.config.Policies {
		for _, policyPath := range policyDef.Files {
			f, err := os.Open(policyPath)
			if err != nil {
				log.Errorf("failed to load policy: %s", policyPath)
				return err
			}

			policy, err := policy.LoadPolicy(f)
			if err != nil {
				log.Errorf("failed to load policy `%s`: %s", policyPath, err)
				return err
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
					return err
				}

				rule, err := a.ruleSet.AddRule(ruleDef.ID, astRule, tags...)
				if err != nil {
					if err, ok := err.(*eval.AstToEvalError); ok {
						log.Errorf("rule syntax error: %s\n%s", err, secl.SprintExprAt(ruleDef.Expression, err.Pos))
					} else {
						log.Errorf("rule compilation error: %s\n%s", err, ruleDef.Expression)
					}
					return err
				}

				if len(rule.GetEventTypes()) == 0 {
					log.Errorf("rule without event specified: %s", ruleDef.Expression)
					return RuleWithoutEventErr
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
		log.Errorf("invalid security agent configuration: %s", err)
		return nil, err
	}

	probe, err := probe.NewProbe(config)
	if err != nil {
		return nil, err
	}

	agent := &Module{
		config: config,
		probe:  probe,
		server: NewEventServer(),
	}

	if err := agent.LoadPolicies(probe.GetModel(), config.Debug); err != nil {
		return nil, err
	}

	return agent, nil
}
