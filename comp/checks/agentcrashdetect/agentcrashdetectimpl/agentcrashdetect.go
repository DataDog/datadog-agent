// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

// Package agentcrashdetectimpl detects agent crashes and reports them
package agentcrashdetectimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/fx"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	compsysconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	"github.com/DataDog/datadog-agent/pkg/util/crashreport"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName          = "agentcrashdetect"
	maxStartupWarnings = 20
	reportedKey        = `lastReported`
)

var (
	// crashdriver included for testing purposes
	ddDrivers = map[string]struct{}{
		"ddnpm":       {}, // NPM/USM driver, used for network monitoring
		"ddprocmon":   {}, // process monitoring driver, used for CWS
		"ddinjector":  {}, // application tracing driver, used for APM
		"crashdriver": {}, // this entry exists only for testing purposes.
	}
	// system probe enabled flags indicating we should be enabled
	enabledflags = []string{
		"windows_crash_detection.enabled",
		"network_config.enabled",
		"service_monitoring_config.enabled",
		"runtime_security_config.enabled",
	}
	// these are vars and not consts so that they can be overridden in
	// the unit tests.
	hive    = registry.LOCAL_MACHINE
	baseKey = `SOFTWARE\Datadog\Datadog Agent\agent_crash_reporting`
)

// AgentBSODStackFrame encapsulates a single frame in a crash stack.
type AgentBSODStackFrame struct {
	InstructionPointer string `json:"ip"`
}

// AgentBSOD for Agent Telemetry reporting
type AgentBSOD struct {
	Date         string                `json:"date"`
	Offender     string                `json:"offender"`
	BugCheck     string                `json:"bugcheck"`
	BugCheckArg1 string                `json:"bugcheckarg1"`
	BugCheckArg2 string                `json:"bugcheckarg2"`
	BugCheckArg3 string                `json:"bugcheckarg3"`
	BugCheckArg4 string                `json:"bugcheckarg4"`
	Frames       []AgentBSODStackFrame `json:"frames"`
	AgentVersion string                `json:"agentversion"`
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAgentCrashComponent))
}

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
	probeconfig           compsysconfig.Component
	atel                  agenttelemetry.Component
}

type agentCrashComponent struct {
}

type dependencies struct {
	fx.In

	Config compsysconfig.Component
	Atel   agenttelemetry.Component

	Lifecycle fx.Lifecycle
}

// Parse parses the check configuration
func (c *WinCrashConfig) Parse(data []byte) error {
	// default values
	c.Enabled = true

	return yaml.Unmarshal(data, c)
}

// Configure accepts the configuration
func (wcd *AgentCrashDetect) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := wcd.CommonConfigure(senderManager, initConfig, data, source)
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

	// check if the crash is related to one of our drivers
	ddFrameFound := false
	for _, frame := range crash.Frames {
		// the dd driver frames should not have resolved symbols
		frameParts := strings.Split(frame, "+")
		if len(frameParts) == 0 {
			continue
		}

		moduleName := strings.ToLower(frameParts[0])
		if _, ok := ddDrivers[moduleName]; ok {
			ddFrameFound = true
			break
		}
	}

	if !ddFrameFound {
		// there was a crash, but not one of our drivers.  don't need to report
		log.Infof("non-dd crash detected %s", crash.Offender)
		return nil
	}

	// Prepare the callstack frames to be crashtracker friendly.
	frames := []AgentBSODStackFrame{}
	for _, f := range crash.Frames {
		frames = append(frames,
			AgentBSODStackFrame{
				InstructionPointer: f,
			})
	}

	log.Infof("Sending crash: %v", formatText(crash))

	bsod := AgentBSOD{
		Date:         crash.DateString,
		Offender:     crash.Offender,
		BugCheck:     crash.BugCheck,
		BugCheckArg1: crash.BugCheckArg1,
		BugCheckArg2: crash.BugCheckArg2,
		BugCheckArg3: crash.BugCheckArg3,
		BugCheckArg4: crash.BugCheckArg4,
		Frames:       frames,
		AgentVersion: crash.AgentVersion,
	}
	var bsodPayload []byte
	bsodPayload, err = json.Marshal(bsod)
	if err != nil {
		return err
	}

	// "agentbsod" is payload type registered with the Agent Telemetry component
	err = wcd.atel.SendEvent("agentbsod", bsodPayload)
	if err != nil {
		return err
	}

	return nil
}

func newAgentCrashComponent(deps dependencies) agentcrashdetect.Component {
	instance := &agentCrashComponent{}
	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			core.RegisterCheck(CheckName, option.New(func() check.Check {
				checkInstance := &AgentCrashDetect{
					CheckBase:   core.NewCheckBase(CheckName),
					instance:    &WinCrashConfig{},
					probeconfig: deps.Config,
					atel:        deps.Atel,
				}
				return checkInstance
			}))
			return nil
		},
	})
	return instance
}

func formatText(c *probe.WinCrashStatus) string {
	baseString := `A system crash was detected.
	The crash occurred at %s.
	The offending moudule is %s.
	The bugcheck code is %s.
	The bugcheck arguments are (%s, %s, %s, %s).
	The Agent version is: %s.
	The callstack is: %v.`
	return fmt.Sprintf(
		baseString,
		c.DateString,
		c.Offender,
		c.BugCheck,
		c.BugCheckArg1,
		c.BugCheckArg2,
		c.BugCheckArg3,
		c.BugCheckArg4,
		c.AgentVersion,
		c.Frames)
}
