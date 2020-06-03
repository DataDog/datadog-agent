package module

import (
	"net"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

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
	probe       *sprobe.Probe
	config      *config.Config
	ruleSet     *eval.RuleSet
	eventServer *EventServer
	grpcServer  *grpc.Server
	listener    net.Listener
}

func (m *Module) Register(server *grpc.Server) error {
	ln, err := net.Listen("unix", m.config.SocketPath)
	if err != nil {
		return errors.Wrap(err, "unable to register security runtime module")
	}
	if err := os.Chmod(m.config.SocketPath, 0700); err != nil {
		return errors.Wrap(err, "unable to register security runtime module")
	}

	m.listener = ln

	go m.grpcServer.Serve(ln)

	m.probe.SetEventHandler(m)
	m.ruleSet.AddListener(m)

	if err := m.probe.Start(); err != nil {
		return err
	}

	if err := m.probe.ApplyRuleSet(m.ruleSet); err != nil {
		log.Warn(err)
	}

	return nil
}

func (m *Module) Close() {
	if m.grpcServer != nil {
		m.grpcServer.Stop()
	}

	if m.listener != nil {
		m.listener.Close()
	}

	m.probe.Stop()
}

// RuleMatch - called by the ruleset when a rule matches
func (m *Module) RuleMatch(rule *eval.Rule, event eval.Event) {
	m.eventServer.SendEvent(rule, event)
}

// EventDiscarderFound - called by the ruleset when a new discarder discovered
func (m *Module) EventDiscarderFound(event eval.Event, field string) {
	m.probe.OnNewDiscarder(event.(*sprobe.Event), field)
}

// HandleEvent - called by the probe when an event arrives from the kernel
func (m *Module) HandleEvent(event *sprobe.Event) {
	m.ruleSet.Evaluate(event)
}

func LoadPolicies(config *config.Config, probe *sprobe.Probe) (*eval.RuleSet, error) {
	var result *multierror.Error

	// Create new ruleset with empty rules and macros
	ruleSet := probe.NewRuleSet(eval.NewOptsWithParams(config.Debug, sprobe.SECLConstants))

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

			// Add the macros to the ruleset and generate macros evaluators
			if err := ruleSet.AddMacros(policy.Macros); err != nil {
				result = multierror.Append(result, err)
			}

			// Add rules to the ruleset and generate rules evaluators
			if err := ruleSet.AddRules(policy.Rules); err != nil {
				result = multierror.Append(result, err)
			}
		}
	}

	return ruleSet, result.ErrorOrNil()
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
		log.Errorf("invalid security module configuration: %s", err)
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

	m := &Module{
		config:      config,
		probe:       probe,
		ruleSet:     ruleSet,
		eventServer: NewEventServer(),
		grpcServer:  grpc.NewServer(),
	}

	api.RegisterSecurityModuleServer(m.grpcServer, m.eventServer)

	return m, nil
}
