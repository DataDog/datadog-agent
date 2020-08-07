// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package module

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	aconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	sapi "github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/statsd"
)

// Module represents the system-probe module for the runtime security agent
type Module struct {
	probe        *sprobe.Probe
	config       *config.Config
	ruleSet      *rules.RuleSet
	eventServer  *EventServer
	grpcServer   *grpc.Server
	listener     net.Listener
	statsdClient *statsd.Client
	rateLimiter  *RateLimiter
}

// Register the runtime security agent module
func (m *Module) Register(httpMux *http.ServeMux) error {
	// force socket cleanup of previous socket not cleanup
	os.Remove(m.config.SocketPath)

	ln, err := net.Listen("unix", m.config.SocketPath)
	if err != nil {
		return errors.Wrap(err, "unable to register security runtime module")
	}
	if err := os.Chmod(m.config.SocketPath, 0700); err != nil {
		return errors.Wrap(err, "unable to register security runtime module")
	}

	m.listener = ln

	go func() {
		if err := m.grpcServer.Serve(ln); err != nil {
			log.Error(err)
		}
	}()

	m.probe.SetEventHandler(m)
	m.ruleSet.AddListener(m)

	go m.statsMonitor(context.Background())

	if err := m.probe.Start(); err != nil {
		return err
	}

	report, err := m.ApplyRuleSet(false)
	if err != nil {
		log.Warn(err)
	}

	// now that the probes have started, run the snapshot functions for the probes that require
	// to fetch the current state of the system (example: mount points probes, process probes, ...)
	if err := m.probe.Snapshot(); err != nil {
		return err
	}

	content, _ := json.MarshalIndent(report, "", "\t")
	log.Debug(string(content))

	return nil
}

// ApplyRuleSet applies the loaded set of rules and returns a report
// of the applied approvers for it. If dryRun is set to true,
// the rules won't be applied but the report will still be returned.
func (m *Module) ApplyRuleSet(dryRun bool) (*probe.Report, error) {
	return m.probe.ApplyRuleSet(m.ruleSet, dryRun)
}

// Close the module
func (m *Module) Close() {
	if m.grpcServer != nil {
		m.grpcServer.Stop()
	}

	if m.listener != nil {
		m.listener.Close()
	}

	m.probe.Stop()
}

// RuleMatch is called by the ruleset when a rule matches
func (m *Module) RuleMatch(rule *eval.Rule, event eval.Event) {
	if m.rateLimiter.Allow(rule.ID) {
		m.eventServer.SendEvent(rule, event)
	} else {
		log.Debugf("Event %s on rule %s was dropped due to rate limiting", event.GetID(), rule.ID)
	}
}

// EventDiscarderFound is called by the ruleset when a new discarder discovered
func (m *Module) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field string) {
	if err := m.probe.OnNewDiscarder(rs, event.(*sprobe.Event), field); err != nil {
		log.Debug(err)
	}
}

// HandleEvent is called by the probe when an event arrives from the kernel
func (m *Module) HandleEvent(event *sprobe.Event) {
	m.ruleSet.Evaluate(event)
}

// LoadPolicies loads the policies listed in the configuration of
// returns the set of the loaded rules
func LoadPolicies(config *config.Config, probe *sprobe.Probe) (*rules.RuleSet, error) {
	var result *multierror.Error

	ruleSet := probe.NewRuleSet(rules.NewOptsWithParams(config.Debug, sprobe.SECLConstants, sprobe.InvalidDiscarders))

	policyFiles, err := ioutil.ReadDir(config.PoliciesDir)
	if err != nil {
		return nil, err
	}

	// Load and parse policies
	for _, policyPath := range policyFiles {
		filename := policyPath.Name()

		// policy path extension check
		if filepath.Ext(filename) != ".policy" {
			log.Debugf("ignoring file `%s` wrong extension `%s`", policyPath.Name(), filepath.Ext(filename))
			continue
		}

		// Open policy path
		f, err := os.Open(filepath.Join(config.PoliciesDir, filename))
		if err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "failed to load policy `%s`", policyPath))
			continue
		}

		// Parse policy file
		policy, err := policy.LoadPolicy(f)
		if err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "failed to load policy `%s`", policyPath))
			continue
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

	return ruleSet, result.ErrorOrNil()
}

func (m *Module) statsMonitor(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.probe.SendStats(m.statsdClient); err != nil {
				log.Debug(err)
			}
			if err := m.rateLimiter.SendStats(m.statsdClient); err != nil {
				log.Debug(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// GetStats returns statistics about the module
func (m *Module) GetStats() map[string]interface{} {
	probeStats, err := m.probe.GetStats()
	if err != nil {
		return nil
	}

	return map[string]interface{}{
		"probe": probeStats,
	}
}

// GetRuleSet returns the set of loaded rules
func (m *Module) GetRuleSet() *rules.RuleSet {
	return m.ruleSet
}

// NewModule instantiates a runtime security system-probe module
func NewModule(cfg *aconfig.AgentConfig) (api.Module, error) {
	config, err := config.NewConfig(cfg)
	if err != nil {
		log.Errorf("invalid security runtime module configuration: %s", err)
		return nil, err
	}

	if !config.Enabled {
		log.Infof("security runtime module disabled")
		return nil, api.ErrNotEnabled
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
		rateLimiter:  NewRateLimiter(ruleSet.ListRuleIDs()),
	}

	sapi.RegisterSecurityModuleServer(m.grpcServer, m.eventServer)

	return m, nil
}
