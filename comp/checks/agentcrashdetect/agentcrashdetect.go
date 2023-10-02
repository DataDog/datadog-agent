// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

// Package agentcrashdetect detects agent crashes and reports them
package agentcrashdetect

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"

	compsysconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"

	"github.com/DataDog/datadog-agent/pkg/internaltelemetry"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/crashreport"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"
)

const (
	crashDetectCheckName = "agentcrashdetect"
	maxStartupWarnings   = 20
	reportedKey          = `lastReported`
)

var (
	// crashdriver included for testing purposes
	ddDrivers = map[string]struct{}{
		"ddnpm":       {},
		"crashdriver": {},
	}
	// system probe enabled flags indicating we should be enabled
	enabledflags = []string{
		"windows_crash_detection.enabled",
		"network_config.enabled",
		"service_monitoring_config.enabled",
	}
	// these are vars and not consts so that they can be overridden in
	// the unit tests.
	hive    = registry.LOCAL_MACHINE
	baseKey = `SOFTWARE\Datadog\Datadog Agent\agent_crash_reporting`
)

// WinCrashConfig is the configuration options for this check
// it is exported so that the yaml parser can read it.
type WinCrashConfig struct {
	Enabled bool `yaml:"enabled"` // placeholder for config
}

// AgentCrashDetect is the core check object; it implements the core check interface
// for running agent checks
type AgentCrashDetect struct {
	core.CheckBase
	instance              *WinCrashConfig
	reporter              *crashreport.WinCrashReporter
	crashDetectionEnabled bool
	tconfig               *traceconfig.AgentConfig
	probeconfig           compsysconfig.Component
}

type agentCrashComponent struct {
	tconfig *traceconfig.AgentConfig
}

type dependencies struct {
	fx.In

	TConfig   comptraceconfig.Component
	SConfig   compsysconfig.Component
	Lifecycle fx.Lifecycle
}

// Parse parses the check configuration
func (c *WinCrashConfig) Parse(data []byte) error {
	// default values
	c.Enabled = true

	return yaml.Unmarshal(data, c)
}

// Configure accepts the configuration
func (wcd *AgentCrashDetect) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := wcd.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}
	wcd.reporter, err = crashreport.NewWinCrashReporter(hive, baseKey)
	if err != nil {
		return err
	}

	// check to see if the wincrashdetect module is enabled.  If not, there's no point
	// in even trying
	for _, k := range enabledflags {
		wcd.crashDetectionEnabled = wcd.probeconfig.GetBool(k)
		if wcd.crashDetectionEnabled {
			break
		}
	}
	if !wcd.crashDetectionEnabled {
		// would prefer to return an error so that the check won't be scheduled.  But if we do that,
		// it will show up as an integration issue in the UI; and this is an "expected" error,
		// not an integration issue.  So just log it here (on startup).  The check will run
		// every time and do nothing.
		log.Infof("Agent Crash Detection module will not run; no required components running")
	}

	return wcd.instance.Parse(initConfig)
}

// Run is called on each check run.
// we're only ever interested in reporting the same crash once.  The reporter.CheckForCrash()
// will handle only reporting the same crash once, and will return nil, even if a crash
// is present, if it's already been reported to this check.
func (wcd *AgentCrashDetect) Run() error {

	if !wcd.crashDetectionEnabled {
		// would prefer to have returned an error at configure time so that the check
		// won't be scheduled.  But if we do that,
		// it will show up as an integration issue in the UI; and this is an "expected" error,
		// not an integration issue.  So just log it here (on startup).  The check will run
		// every time and do nothing.

		// No sysprobe crash detection module would be enabled.  So don't try
		return nil
	}

	crash, err := wcd.reporter.CheckForCrash()
	if err != nil {
		return err
	}
	if crash == nil {
		// no crash to send
		return nil
	}

	// check to see if the crash is one of ours
	offender := strings.Split(crash.Offender, "+")[0]
	if _, ok := ddDrivers[offender]; !ok {
		log.Infof("non-dd crash detected %s", offender)
		// there was a crash, but not one of our drivers.  don't need to report
		return nil
	}

	log.Infof("Sending crash: %v", formatText(crash))
	lts := internaltelemetry.NewLogTelemetrySender(wcd.tconfig, "ddnpm", "go")
	lts.SendLog("WARN", formatText(crash))
	return nil
}

func newAgentCrashComponent(deps dependencies) Component {
	instance := &agentCrashComponent{}
	instance.tconfig = deps.TConfig.Object()
	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			core.RegisterCheck(crashDetectCheckName, func() check.Check {
				checkInstance := &AgentCrashDetect{
					CheckBase:   core.NewCheckBase(crashDetectCheckName),
					instance:    &WinCrashConfig{},
					tconfig:     instance.tconfig,
					probeconfig: deps.SConfig,
				}
				return checkInstance
			})
			return nil
		},
	})
	return instance
}

func formatText(c *probe.WinCrashStatus) string {
	baseString := `A system crash was detected.
	The crash occurred at %s.
	The offending moudule is %s.
	The bugcheck code is %s`
	return fmt.Sprintf(baseString, c.DateString, c.Offender, c.BugCheck)
}
