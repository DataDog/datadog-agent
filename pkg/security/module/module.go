package module

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

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
	"github.com/DataDog/datadog-go/statsd"
)

type Module struct {
	probe        *sprobe.Probe
	config       *config.Config
	ruleSet      *eval.RuleSet
	eventServer  *EventServer
	grpcServer   *grpc.Server
	listener     net.Listener
	statsdClient *statsd.Client
	rateLimiter *RateLimiter
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

	go m.statsMonitor(context.Background())

	if err := m.probe.Start(); err != nil {
		return err
	}

	if err := m.probe.ApplyRuleSet(m.ruleSet); err != nil {
		log.Warn(err)
	}

	// now that the probes have started, run the snapshot functions for the probes that require
	// to fetch the current state of the system (example: mount points probes, process probes, ...)
	if err := m.probe.Snapshot(); err != nil {
		return err
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
	if m.rateLimiter.Allow(rule.ID) {
		m.eventServer.SendEvent(rule, event)
	} else {
		log.Debugf("Event %s on rule %s was dropped due to rate limiting", event.GetID(), rule.ID)
	}
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

func (m *Module) statsMonitor(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.probe.SendStats(m.statsdClient)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Module) GetStats() map[string]interface{} {
	probeStats, err := m.probe.GetStats()
	if err != nil {
		return nil
	}

	return map[string]interface{}{
		"probe": probeStats,
	}
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

	var statsdClient *statsd.Client
	if cfg != nil {
		statsdAddr := os.Getenv("STATSD_URL")
		if statsdAddr == "" {
			statsdAddr = fmt.Sprintf("%s:%d", cfg.StatsdHost, cfg.StatsdPort)
		}
		if statsdClient, err = statsd.New(statsdAddr); err != nil {
			return nil, err
		}
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
		config:       config,
		probe:        probe,
		ruleSet:      ruleSet,
		eventServer:  NewEventServer(),
		grpcServer:   grpc.NewServer(),
		statsdClient: statsdClient,
		rateLimiter: NewRateLimiter(ruleSet.ListRuleIDs()),
	}

	api.RegisterSecurityModuleServer(m.grpcServer, m.eventServer)

	return m, nil
}
