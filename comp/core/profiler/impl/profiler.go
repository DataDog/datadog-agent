// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package profilerimpl implements the profiler component interface
package profilerimpl

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	profilercomp "github.com/DataDog/datadog-agent/comp/core/profiler/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// Requires defines the dependencies for the profiler component
type Requires struct {
	compdef.In

	SettingsComponent settings.Component
	Config            config.Component
	SysProbeConfig    sysprobeconfig.Component
	IPCClient         ipc.HTTPClient
}

// Provides defines the output of the profiler component
type Provides struct {
	compdef.Out

	Comp          profilercomp.Component
	FlareProvider flaretypes.Provider
}

type profiler struct {
	settingsComponent settings.Component
	cfg               config.Component
	sysProbeCfg       sysprobeconfig.Component
	ipcClient         ipc.HTTPClient
}

// ReadProfileData gathers and returns pprof server output for a variety of agent services.
//
// Will always attempt to read the pprof of core-agent and security-agent, and will optionally try to read information for
// process-agent, trace-agent, and system-probe if those systems are detected as enabled.
//
// This function is exposed via the public api to support the flare generation cli command. While the goal
// is to move the profiling component completely into a flare provider, the existing architecture
// expects an explicit and pre-emptive profiling run before the flare logic is properly called.
func (p profiler) ReadProfileData(seconds int, logFunc func(log string, params ...interface{}) error) (flaretypes.ProfileData, error) {
	type agentProfileCollector func(service string) error

	pdata := flaretypes.ProfileData{}

	type pprofGetter func(path string) ([]byte, error)
	tcpGet := func(portConfig string, onHTTPS bool) pprofGetter {
		endpoint := url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(p.cfg.GetInt(portConfig))),
			Path:   "/debug/pprof",
		}
		if onHTTPS {
			endpoint.Scheme = "https"
		}
		return func(path string) ([]byte, error) {
			return p.ipcClient.Get(endpoint.String()+path, ipchttp.WithLeaveConnectionOpen)
		}
	}

	serviceProfileCollector := func(get func(url string) ([]byte, error), seconds int) agentProfileCollector {
		return func(service string) error {
			_ = logFunc("Getting a %ds profile snapshot from %s.", seconds, service)

			for _, prof := range []struct{ name, path string }{
				{
					// 1st heap profile
					name: service + "-1st-heap.pprof",
					path: "/heap",
				},
				{
					// CPU profile
					name: service + "-cpu.pprof",
					path: fmt.Sprintf("/profile?seconds=%d", seconds),
				},
				{
					// 2nd heap profile
					name: service + "-2nd-heap.pprof",
					path: "/heap",
				},
				{
					// mutex profile
					name: service + "-mutex.pprof",
					path: "/mutex",
				},
				{
					// goroutine blocking profile
					name: service + "-block.pprof",
					path: "/block",
				},
				{
					// Trace
					name: service + ".trace",
					path: fmt.Sprintf("/trace?seconds=%d", seconds),
				},
			} {
				b, err := get(prof.path)
				if err != nil {
					return err
				}
				pdata[prof.name] = b
			}
			return nil
		}
	}

	agentCollectors := map[string]agentProfileCollector{}
	if p.coreAgentEnabled() {
		agentCollectors["core"] = serviceProfileCollector(tcpGet("expvar_port", false), seconds)
	}

	if p.securityAgentEnabled() {
		agentCollectors["security-agent"] = serviceProfileCollector(tcpGet("security_agent.expvar_port", false), seconds)
	}

	if p.processAgentEnabled() {
		agentCollectors["process"] = serviceProfileCollector(tcpGet("process_config.expvar_port", false), seconds)
	}

	if p.apmEnabled() {
		traceCpusec := p.apmTraceSeconds(seconds)
		agentCollectors["trace"] = serviceProfileCollector(tcpGet("apm_config.debug.port", true), traceCpusec)
	}

	if p.sysProbeEnabled() {
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: sysprobeclient.DialContextFunc(p.sysProbeCfg.GetString("system_probe_config.sysprobe_socket")),
			},
		}

		sysProbeGet := func() pprofGetter {
			return func(path string) ([]byte, error) {
				var buf bytes.Buffer
				pprofURL := sysprobeclient.DebugURL("/pprof" + path)
				req, err := http.NewRequest(http.MethodGet, pprofURL, &buf)
				if err != nil {
					return nil, err
				}

				res, err := client.Do(req)
				if err != nil {
					return nil, err
				}
				defer res.Body.Close()

				return io.ReadAll(res.Body)
			}
		}

		agentCollectors["system-probe"] = serviceProfileCollector(sysProbeGet(), seconds)
	}

	var errs error
	for name, callback := range agentCollectors {
		if err := callback(name); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("error collecting %s agent profile: %v", name, err))
		}
	}

	return pdata, errs
}

func (p profiler) setProfilerSetting(settingName string, newValue int, fb flaretypes.FlareBuilder) func() {
	oldValue, err := p.settingsComponent.GetRuntimeSetting(settingName)
	if err != nil {
		_ = fb.Logf("Unable to access %s setting, ignoring value provided by flare args: %v", settingName, err)

		return nil
	} else if newValue <= 0 || newValue == oldValue {
		return nil
	}

	err = p.settingsComponent.SetRuntimeSetting(settingName, newValue, model.SourceAgentRuntime)
	if err != nil {
		_ = fb.Logf("Unable to set the %s: %v", settingName, err)
		return nil
	}

	return func() {
		nErr := p.settingsComponent.SetRuntimeSetting(settingName, oldValue, model.SourceAgentRuntime)
		if nErr != nil {
			_ = fb.Logf("Unable to reset the %s back to its original value: %v", settingName, nErr)
		}
	}
}

// Currently flare args are only populated (and this function is only enabled) via
// the RC flare generation flow. The goal is to shift other flare generation flows
// to utilize this provider over time, which will require additional plumbing. For
// example, the flare api call must be expanded to take in an optional profile duration
// before the cli command can be fully ported over.
func (p profiler) fillFlare(fb flaretypes.FlareBuilder) error {
	duration := fb.GetFlareArgs().ProfileDuration

	if duration <= 0 {
		_ = fb.Logf("Profiling retrieval has been disabled via an unset duration, exiting profile flare filler")
		return nil
	}

	blockingRate := fb.GetFlareArgs().ProfileBlockingRate
	bDeferFunc := p.setProfilerSetting("runtime_block_profile_rate", blockingRate, fb)
	if bDeferFunc != nil {
		defer bDeferFunc()
	}

	mutexFraction := fb.GetFlareArgs().ProfileMutexFraction
	mDeferFunc := p.setProfilerSetting("runtime_mutex_profile_fraction", mutexFraction, fb)
	if mDeferFunc != nil {
		defer mDeferFunc()
	}

	pdata, err := p.ReadProfileData(int(duration.Seconds()), fb.Logf)

	// For legacy reasons it's not unexpected to get partial errors from ReadProfileData, record them in the logs
	// and persist whatever data we did get
	if err != nil {
		_ = fb.Logf("Errors encountered generating flare profiles: %s", err)
	}
	for name, data := range pdata {
		fb.AddFileWithoutScrubbing(filepath.Join("profiles", name), data)
	}

	return nil
}

// Currently the core agent is always considered enabled
func (profiler) coreAgentEnabled() bool {
	return true
}

// Currently the security agent is always considered enabled
func (profiler) securityAgentEnabled() bool {
	return true
}

func (p profiler) processAgentEnabled() bool {
	processChecksEnabled := p.cfg.GetBool("process_config.enabled") ||
		p.cfg.GetBool("process_config.container_collection.enabled") ||
		p.cfg.GetBool("process_config.process_collection.enabled")
	processChecksInProcessAgent := !p.cfg.GetBool("process_config.run_in_core_agent.enabled") &&
		processChecksEnabled
	npmEnabled := p.sysProbeCfg.GetBool("network_config.enabled")
	usmEnabled := p.sysProbeCfg.GetBool("service_monitoring_config.enabled")

	return processChecksInProcessAgent || npmEnabled || usmEnabled
}

func (p profiler) apmEnabled() bool {
	return p.cfg.GetBool("apm_config.enabled")
}

func (p profiler) sysProbeEnabled() bool {
	return p.sysProbeCfg.GetBool("system_probe_config.enabled")
}

func (p profiler) timeout(fb flaretypes.FlareBuilder) time.Duration {
	d := fb.GetFlareArgs().ProfileDuration
	if d <= 0 {
		return 0
	}
	var timeout time.Duration

	// pprof's /profile + /trace both require [duration] seconds to track, so each agent requires
	// at least 2*[duration] of runtime
	if p.coreAgentEnabled() {
		timeout += 2 * d
	}
	if p.securityAgentEnabled() {
		timeout += 2 * d
	}
	if p.processAgentEnabled() {
		timeout += 2 * d
	}
	if p.apmEnabled() {
		apmSeconds := p.apmTraceSeconds(int(d.Seconds()))
		timeout += 2 * (time.Duration(apmSeconds) * time.Second)
	}
	if p.sysProbeEnabled() {
		timeout += 2 * d
	}

	processOverhead := p.cfg.GetDuration("flare.profile_overhead_runtime")

	return timeout + processOverhead
}

func (p profiler) apmTraceSeconds(seconds int) int {
	traceCpusec := p.cfg.GetInt("apm_config.receiver_timeout")
	if traceCpusec > seconds {
		// do not exceed requested duration
		traceCpusec = seconds
	} else if traceCpusec <= 0 {
		// default to 4s as maximum connection timeout of trace-agent HTTP server is 5s by default
		traceCpusec = 4
	}

	return traceCpusec
}

// NewComponent creates a new Profiler component
func NewComponent(req Requires) (Provides, error) {
	p := profiler{
		settingsComponent: req.SettingsComponent,
		cfg:               req.Config,
		sysProbeCfg:       req.SysProbeConfig,
		ipcClient:         req.IPCClient,
	}
	return Provides{
		Comp:          p,
		FlareProvider: flaretypes.NewProviderWithTimeout(p.fillFlare, p.timeout),
	}, nil
}
