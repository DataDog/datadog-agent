package agent

import (
	"fmt"
	"log"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type Rule struct {
	evaluator *eval.RuleEvaluator
	astRule   *ast.Rule
	name      string
}

type SignalListener interface {
	HandleSignal(rule *Rule, ctx *eval.Context)
}

type Agent struct {
	probe           *probe.Probe
	config          *config.Config
	rules           []*Rule
	signalListeners []SignalListener
}

func (a *Agent) Start() error {
	return a.probe.Start()
}

func (a *Agent) Stop() error {
	a.probe.Stop()
	return nil
}

func (a *Agent) AddSignalListener(listener SignalListener) {
	a.signalListeners = append(a.signalListeners, listener)
}

func (a *Agent) TriggerSignal(rule *Rule, ctx *eval.Context) {
	log.Printf("Rule %s was triggered (event: %+v)\n", rule.name, spew.Sdump(ctx.Event))

	for _, listener := range a.signalListeners {
		listener.HandleSignal(rule, ctx)
	}
}

func (a *Agent) HandleEvent(event interface{}) {
	context := &eval.Context{Event: &model.Event{}}

	if dentryEvent, ok := event.(*model.DentryEvent); ok {
		context.Event.DentryEvent = dentryEvent
	}

	for _, rule := range a.rules {
		if rule.evaluator.Eval(context) {
			a.TriggerSignal(rule, context)
		}
	}
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
				astRule, err := ast.ParseRule(ruleDef.Expression)
				if err != nil {
					return errors.Wrap(err, "invalid rule")
				}

				evaluator, err := eval.RuleToEvaluator(astRule, a.config.Debug)
				if err != nil {
					return err
				}

				a.rules = append(a.rules, &Rule{
					astRule:   astRule,
					evaluator: evaluator,
					name:      ruleDef.Name,
				})
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

	agent := &Agent{
		config: config,
	}

	agent.probe, err = probe.NewProbe(agent)
	if err != nil {
		return nil, err
	}

	if err := agent.LoadPolicies(); err != nil {
		return nil, err
	}

	return agent, nil
}
