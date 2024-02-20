// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && (functionaltests || stresstests)

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
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
)

var (
	testActivityDumpDuration             = time.Second * 30
	testActivityDumpLoadControllerPeriod = time.Second * 10
)

const testPolicy = `---
version: 1.2.3

macros:
{{range $Macro := .Macros}}
  - id: {{$Macro.ID}}
    expression: >-
      {{$Macro.Expression}}
{{end}}

rules:
{{range $Rule := .Rules}}
  - id: {{$Rule.ID}}
    version: {{$Rule.Version}}
    expression: >-
      {{$Rule.Expression}}
    tags:
{{- range $Tag, $Val := .Tags}}
      {{$Tag}}: {{$Val}}
{{- end}}
    actions:
{{- range $Action := .Actions}}
{{- if $Action.Set}}
      - set:
          name: {{$Action.Set.Name}}
		  {{- if $Action.Set.Value}}
          value: {{$Action.Set.Value}}
          {{- else if $Action.Set.Field}}
          field: {{$Action.Set.Field}}
          {{- end}}
          scope: {{$Action.Set.Scope}}
          append: {{$Action.Set.Append}}
{{- end}}
{{- if $Action.Kill}}
      - kill:
          {{- if $Action.Kill.Signal}}
          signal: {{$Action.Kill.Signal}}
          {{- end}}
{{- end}}
{{- end}}
{{end}}
`

const testConfig = `---
log_level: DEBUG

event_monitoring_config:
  remote_tagger: false
  custom_sensitive_words:
    - "*custom*"
  network:
    enabled: true
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
  erpc_dentry_resolution_enabled: {{ .ErpcDentryResolutionEnabled }}
  map_dentry_resolution_enabled: {{ .MapDentryResolutionEnabled }}
  envs_with_value:
  {{range .EnvsWithValue}}
    - {{.}}
  {{end}}

runtime_security_config:
  enabled: {{ .RuntimeSecurityEnabled }}
  internal_monitoring:
    enabled: true
  remote_configuration:
    enabled: false
  sbom:
    enabled: {{ .SBOMEnabled }}
  fim_enabled: {{ .FIMEnabled }}
  activity_dump:
    enabled: {{ .EnableActivityDump }}
{{if .EnableActivityDump}}
    rate_limiter: {{ .ActivityDumpRateLimiter }}
    tag_rules:
      enabled: {{ .ActivityDumpTagRules }}
    dump_duration: {{ .ActivityDumpDuration }}
    {{if .ActivityDumpLoadControllerPeriod }}
    load_controller_period: {{ .ActivityDumpLoadControllerPeriod }}
    {{end}}
    {{if .ActivityDumpCleanupPeriod }}
    cleanup_period: {{ .ActivityDumpCleanupPeriod }}
    {{end}}
    {{if .ActivityDumpLoadControllerTimeout }}
    min_timeout: {{ .ActivityDumpLoadControllerTimeout }}
    {{end}}
    traced_cgroups_count: {{ .ActivityDumpTracedCgroupsCount }}
    traced_event_types:   {{range .ActivityDumpTracedEventTypes}}
    - {{.}}
    {{end}}
    local_storage:
      output_directory: {{ .ActivityDumpLocalStorageDirectory }}
      compression: {{ .ActivityDumpLocalStorageCompression }}
      formats: {{range .ActivityDumpLocalStorageFormats}}
      - {{.}}
      {{end}}
{{end}}
  security_profile:
    enabled: {{ .EnableSecurityProfile }}
{{if .EnableSecurityProfile}}
    dir: {{ .SecurityProfileDir }}
    watch_dir: {{ .SecurityProfileWatchDir }}
    anomaly_detection:
      enabled: true
      default_minimum_stable_period: {{.AnomalyDetectionDefaultMinimumStablePeriod}}
      minimum_stable_period:
        exec: {{.AnomalyDetectionMinimumStablePeriodExec}}
        dns: {{.AnomalyDetectionMinimumStablePeriodDNS}}
      workload_warmup_period: {{.AnomalyDetectionWarmupPeriod}}
{{end}}

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
  ebpfless:
    enabled: {{.EBPFLessEnabled}}
`

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
}

var testMod *testModule
var commonCfgDir string

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
	if _, err = setTestPolicy(commonCfgDir, macroDefs, ruleDefs); err != nil {
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
