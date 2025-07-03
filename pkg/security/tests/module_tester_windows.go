// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-multierror"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	rulesmodule "github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const testConfig = `---
log_level: DEBUG

event_monitoring_config:
  custom_sensitive_words:
    - "*custom*"
  flush_discarder_window: 0
{{if .DisableFilters}}
  enable_kernel_filters: false
{{end}}
{{if .DisableApprovers}}
  enable_approvers: false
{{end}}
{{if .DisableDiscarders}}
  enable_discarders: false
{{end}}
  envs_with_value:
  {{range .EnvsWithValue}}
    - {{.}}
  {{end}}

runtime_security_config:
  enabled: {{ .RuntimeSecurityEnabled }}
{{ if gt .EventServerRetention 0 }}
  event_server:
    retention: {{ .EventServerRetention }}
{{ end }}
  internal_monitoring:
    enabled: true
  remote_configuration:
    enabled: false
  sbom:
    enabled: {{ .SBOMEnabled }}
  fim_enabled: {{ .FIMEnabled }}

  self_test:
    enabled: false

  policies:
    dir: {{.TestPoliciesDir}}
  log_patterns:
  {{range .LogPatterns}}
    - "{{.}}"
  {{end}}
  log_tags:
  {{range .LogTags}}
    - {{.}}
  {{end}}
  enforcement:
    exclude_binaries:
      - {{ .EnforcementExcludeBinary }}
    disarmer:
      container:
        enabled: {{.EnforcementDisarmerContainerEnabled}}
        max_allowed: {{.EnforcementDisarmerContainerMaxAllowed}}
        period: {{.EnforcementDisarmerContainerPeriod}}
      executable:
        enabled: {{.EnforcementDisarmerExecutableEnabled}}
        max_allowed: {{.EnforcementDisarmerExecutableMaxAllowed}}
        period: {{.EnforcementDisarmerExecutablePeriod}}
`

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
}

func newTestModule(t testing.TB, macroDefs []*rules.MacroDefinition, ruleDefs []*rules.RuleDefinition, fopts ...optFunc) (*testModule, error) {

	var opts tmOpts
	for _, opt := range fopts {
		opt(&opts)
	}

	if commonCfgDir == "" {
		cd, err := os.MkdirTemp("", "test-cfgdir")
		if err != nil {
			fmt.Println(err)
		}
		commonCfgDir = cd
	}

	st, err := newSimpleTest(t, macroDefs, ruleDefs, opts.dynamicOpts.testDir)
	if err != nil {
		return nil, err
	}
	if err := setTestPolicy(commonCfgDir, macroDefs, ruleDefs); err != nil {
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
			StatsdClient:       statsdClient,
			DontDiscardRuntime: true,
		},
	}
	if opts.staticOpts.tagger != nil {
		emopts.ProbeOpts.Tagger = opts.staticOpts.tagger
	} else {
		emopts.ProbeOpts.Tagger = NewFakeTaggerDifferentImageNames()
	}

	ipcComp := ipcmock.New(t)

	testMod.eventMonitor, err = eventmonitor.NewEventMonitor(emconfig, secconfig, ipcComp, emopts)
	if err != nil {
		return nil, err
	}
	testMod.probe = testMod.eventMonitor.Probe

	var ruleSetloadedErr *multierror.Error
	if !opts.staticOpts.disableRuntimeSecurity {
		compression := logscompression.NewComponent()
		cws, err := module.NewCWSConsumer(testMod.eventMonitor, secconfig.RuntimeSecurity, nil, module.Opts{EventSender: testMod}, compression, ipcComp)
		if err != nil {
			return nil, fmt.Errorf("failed to create module: %w", err)
		}
		cws.PrepareForFunctionalTests()

		testMod.cws = cws
		testMod.ruleEngine = cws.GetRuleEngine()

		testMod.eventMonitor.RegisterEventConsumer(cws)

		testMod.ruleEngine.SetRulesetLoadedCallback(func(rs *rules.RuleSet, err *multierror.Error) {
			ruleSetloadedErr = err
			log.Infof("Adding test module as listener")
			rs.AddListener(testMod)
		})
	}

	// listen to probe event
	if err := testMod.probe.AddEventHandler(testMod); err != nil {
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
		t.Cleanup(func() {
			testMod.RegisterRuleEventHandler(nil)
		})
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
	tm.eventMonitor.Close()
}

func (tm *testModule) writePlatformSpecificTimeoutError(b *strings.Builder) {
}
