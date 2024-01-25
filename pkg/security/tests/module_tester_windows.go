// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && (functionaltests || stresstests)

// Package tests holds tests related files
package tests

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	spconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	rulesmodule "github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/go-multierror"
)

type onRuleHandler func(*model.Event, *rules.Rule)
type onProbeEventHandler func(*model.Event)
type onCustomSendEventHandler func(*rules.Rule, *events.CustomEvent)
type onSendEventHandler func(*rules.Rule, *model.Event)
type onDiscarderPushedHandler func(event eval.Event, field eval.Field, eventType eval.EventType) bool

type eventHandlers struct {
	sync.RWMutex
	onRuleMatch       onRuleHandler
	onProbeEvent      onProbeEventHandler
	onCustomSendEvent onCustomSendEventHandler
	onSendEvent       onSendEventHandler
	onDiscarderPushed onDiscarderPushedHandler
}

type testModule struct {
	sync.RWMutex
	secconfig     *secconfig.Config
	opts          tmOpts
	st            *simpleTest
	t             testing.TB
	eventMonitor  *eventmonitor.EventMonitor
	cws           *module.CWSConsumer
	probe         *sprobe.Probe
	eventHandlers eventHandlers
	cmdWrapper    cmdWrapper
	statsdClient  *statsdclient.StatsdClient
	proFile       *os.File
	ruleEngine    *rulesmodule.RuleEngine
	//tracePipe     *tracePipeLogger
}

var testMod *testModule
var commonCfgDir string

func newTestModule(t testing.TB, macroDefs []*rules.MacroDefinition, ruleDefs []*rules.RuleDefinition, fopts ...optFunc) (*testModule, error) {

	var opts tmOpts
	for _, opt := range fopts {
		opt(&opts)
	}

	st, err := newSimpleTest(t, macroDefs, ruleDefs, opts.dynamicOpts.testDir)
	if err != nil {
		return nil, err
	}
	statsdClient := statsdclient.NewStatsdClient()

	emconfig, secconfig, err := genTestConfigs(commonCfgDir, opts.staticOpts)
	if err != nil {
		return nil, err
	}

	cmdWrapper := newStdCmdWrapper()

	t.Log("Instantiating a new security module")

	testMod = &testModule{
		secconfig:     secconfig,
		opts:          opts,
		st:            st,
		t:             t,
		cmdWrapper:    cmdWrapper,
		statsdClient:  statsdClient,
		proFile:       nil,
		eventHandlers: eventHandlers{},
	}

	emopts := eventmonitor.Opts{
		StatsdClient: statsdClient,
		ProbeOpts: sprobe.Opts{
			StatsdClient: statsdClient,
		},
	}
	testMod.eventMonitor, err = eventmonitor.NewEventMonitor(emconfig, secconfig, emopts)
	if err != nil {
		return nil, err
	}
	testMod.probe = testMod.eventMonitor.Probe

	var ruleSetloadedErr *multierror.Error
	if !opts.staticOpts.disableRuntimeSecurity {
		cws, err := module.NewCWSConsumer(testMod.eventMonitor, secconfig.RuntimeSecurity, module.Opts{EventSender: testMod})
		if err != nil {
			return nil, fmt.Errorf("failed to create module: %w", err)
		}
		testMod.cws = cws
		testMod.ruleEngine = cws.GetRuleEngine()

		testMod.eventMonitor.RegisterEventConsumer(cws)

		testMod.ruleEngine.SetRulesetLoadedCallback(func(es *rules.EvaluationSet, err *multierror.Error) {
			ruleSetloadedErr = err
			log.Infof("Adding test module as listener")
			for _, ruleSet := range es.RuleSets {
				ruleSet.AddListener(testMod)
			}
		})
	}

	// listen to probe event
	if err := testMod.probe.AddFullAccessEventHandler(testMod); err != nil {
		return nil, err
	}

	testMod.probe.AddDiscarderPushedCallback(testMod.NotifyDiscarderPushedCallback)

	if err := testMod.eventMonitor.Init(); err != nil {
		return nil, fmt.Errorf("failed to init module: %w", err)
	}

	if opts.staticOpts.preStartCallback != nil {
		opts.staticOpts.preStartCallback(testMod)
	}

	if opts.staticOpts.snapshotRuleMatchHandler != nil {
		testMod.RegisterRuleEventHandler(func(e *model.Event, r *rules.Rule) {
			opts.staticOpts.snapshotRuleMatchHandler(testMod, e, r)
		})
		defer testMod.RegisterRuleEventHandler(nil)
	}

	if err := testMod.eventMonitor.Start(); err != nil {
		return nil, fmt.Errorf("failed to start module: %w", err)
	}

	if ruleSetloadedErr.ErrorOrNil() != nil {
		defer testMod.Close()
		return nil, ruleSetloadedErr.ErrorOrNil()
	}

	return testMod, nil

}

func (tm *testModule) Close() {
}

// NewTimeoutError returns a new timeout error with the metrics collected during the test
func (tm *testModule) NewTimeoutError() ErrTimeout {
	var msg strings.Builder

	msg.WriteString("timeout, details: ")

	events := tm.ruleEngine.StopEventCollector()
	if len(events) != 0 {
		msg.WriteString("\nevents evaluated:\n")

		for _, event := range events {
			msg.WriteString(fmt.Sprintf("%s (eval=%v) {\n", event.Type, event.EvalResult))
			for field, value := range event.Fields {
				msg.WriteString(fmt.Sprintf("\t%s=%v,\n", field, value))
			}
			msg.WriteString("}\n")
		}
	}

	return ErrTimeout{msg.String()}
}

func genTestConfigs(cfgDir string, opts testOpts) (*emconfig.Config, *secconfig.Config, error) {
	buffer := new(bytes.Buffer)
	_, sysprobeConfigName, err := func() (string, string, error) {
		ddConfig, err := os.OpenFile(path.Join(cfgDir, "datadog.yaml"), os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return "", "", err
		}
		defer ddConfig.Close()

		sysprobeConfig, err := os.Create(path.Join(cfgDir, "system-probe.yaml"))
		if err != nil {
			return "", "", err
		}
		defer sysprobeConfig.Close()

		_, err = io.Copy(sysprobeConfig, buffer)
		if err != nil {
			return "", "", err
		}
		return ddConfig.Name(), sysprobeConfig.Name(), nil
	}()
	if err != nil {
		return nil, nil, err
	}
	spconfig, err := spconfig.New(sysprobeConfigName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	emconfig := emconfig.NewConfig(spconfig)

	secconfig, err := secconfig.NewConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	return emconfig, secconfig, nil
}
