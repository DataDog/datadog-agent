// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.
//go:build windows

// package agentcrashdetect detects agent crashes and reports them
package agentcrashdetect

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"

	//"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"

	//comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"

	// configComponent "github.com/DataDog/datadog-agent/comp/core/config"

	// "github.com/DataDog/datadog-agent/pkg/config"
	//"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/internaltelemetry"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"
)

const (
	crashDetectCheckName = "agentcrashdetect"
	maxStartupWarnings   = 20
	reportedKey          = `lastReported`
)

var (
	// these are vars and not consts so that they can be overridden in
	// the unit tests.
	hive    = registry.LOCAL_MACHINE
	baseKey = `SOFTWARE\Datadog\Datadog Agent\windows_agent_crash_reporting`
)

// Config is the configuration options for this check
// it is exported so that the yaml parser can read it.
type WinCrashConfig struct {
	Enabled bool `yaml:"enabled"` // placeholder for config
}

type AgentCrashDetect struct {
	core.CheckBase
	instance         *WinCrashConfig
	hasRunOnce       bool
	startupWarnCount int
	tconfig          *traceconfig.AgentConfig
}

type agentCrashComponent struct {
	//aconfig config.ConfigReader
	tconfig *traceconfig.AgentConfig
	//fx.Out
	//Component
}

type dependencies struct {
	fx.In

	//TraceConfigComponent comptraceconfig.Component
	//Config configComponent.Component
	TConfig   comptraceconfig.Component
	Lifecycle fx.Lifecycle
}

// Parse parses the check configuration
func (c *WinCrashConfig) Parse(data []byte) error {
	// default values
	c.Enabled = true

	return yaml.Unmarshal(data, c)
}

func (wcd *AgentCrashDetect) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := wcd.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	return wcd.instance.Parse(initConfig)
}

func (wcd *AgentCrashDetect) handleStartupError(err error) error {
	if retry.IsErrWillRetry(err) {
		wcd.startupWarnCount++
		// this is an expected error, the check run will occur before the system probe
		// comes up.  However, it should resolve.  If the number of these exceeds a
		// given threshold, report as an error anyway
		if wcd.startupWarnCount > maxStartupWarnings {
			return err
		}
		return nil
	}
	return err
}
func (wcd *AgentCrashDetect) Run() error {

	// only do this once; there's no point in doing it after the first one
	if wcd.hasRunOnce {
		log.Infof("check already run")
		return nil
	}

	/*
	 * originally did this with a sync.once.  The problem is the check is run prior to the
	 * system probe being successfully started.  This is OK; we just need to detect the BSOD
	 * on first available run.
	 *
	 * so keep running the check until we are able to talk to system probe, after which
	 * we don't need to run any more
	 */
	// for now send every thirty secs
	//wcd.hasRunOnce = true

	/*
	 * send a test message
	 */

	log.Infof("sending message")
	lts := internaltelemetry.NewLogTelemetrySender(wcd.tconfig)
	lts.SendLog("WARN", "test log telemetry message")
	return nil
}

func newAgentCrashComponent(deps dependencies) Component {
	instance := &agentCrashComponent{}
	//instance.aconfig = deps.Config.Object()
	instance.tconfig = deps.TConfig.Object()
	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			core.RegisterCheck(crashDetectCheckName, func() check.Check {
				checkInstance := &AgentCrashDetect{
					CheckBase:  core.NewCheckBase(crashDetectCheckName),
					instance:   &WinCrashConfig{},
					hasRunOnce: false,
					tconfig:    instance.tconfig,
				}
				return checkInstance
			})
			return nil
		},
	})
	return instance
}
