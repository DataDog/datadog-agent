// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows && npm

package networktracerimpl

import (
	"os"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/messagestrings"
)

// Requires defines the dependencies for the networktracer component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Telemetry      telemetry.Component
	Statsd         ddgostatsd.ClientInterface

	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
}

type networkTracerComp struct {
	createFn func() (types.SystemProbeModule, error)
}

func (n *networkTracerComp) Name() sysconfigtypes.ModuleName {
	return sysconfig.NetworkTracerModule
}

func (n *networkTracerComp) ConfigNamespaces() []string {
	return networkTracerModuleConfigNamespaces
}

func (n *networkTracerComp) Create() (types.SystemProbeModule, error) {
	return n.createFn()
}

// NewComponent creates a new networktracer component
func NewComponent(reqs Requires) (Provides, error) {
	nt := &networkTracerComp{
		createFn: func() (types.SystemProbeModule, error) {
			return createNetworkTracerModule(reqs.Telemetry, reqs.Statsd)
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: nt},
		Comp:   nt,
	}
	return provides, nil
}

type networkTracer struct {
	tracer       *tracer.Tracer
	cfg          *networkconfig.Config
	syscfg       sysprobeconfig.Component
	restartTimer *time.Timer
}

// Close will stop all system probe activities
func (nt *networkTracer) Close() {
	nt.tracer.Stop()
}

func createNetworkTracerModule(telemetry telemetry.Component, statsd ddgostatsd.ClientInterface) (types.SystemProbeModule, error) {
	ncfg := networkconfig.New()

	// Checking whether the current OS + kernel version is supported by the tracer
	if supported, err := tracer.IsTracerSupportedByOS(ncfg.ExcludedBPFLinuxVersions); !supported {
		return nil, fmt.Errorf("%w: %s", ErrSysprobeUnsupported, err)
	}

	if ncfg.NPMEnabled {
		log.Info("enabling network performance monitoring (NPM)")
	}
	if ncfg.ServiceMonitoringEnabled {
		log.Info("enabling universal service monitoring (USM)")
	}

	t, err := tracer.NewTracer(ncfg, telemetry, statsd)
	if err != nil {
		return nil, err
	}

	return &networkTracer{
		tracer: t,
		cfg:    ncfg,
	}, nil
}

func (nt *networkTracer) platformRegister(httpMux *module.Router) error {
	if !nt.cfg.DirectSend {
		nt.restartTimer = time.AfterFunc(inactivityRestartDuration, func() {
			log.Criticalf("%v since the process-agent last queried for data. It may not be configured correctly and/or running. Exiting system-probe to save system resources.", inactivityRestartDuration)
			winutil.LogEventViewer(config.ServiceName, messagestrings.MSG_SYSPROBE_RESTART_INACTIVITY, inactivityRestartDuration.String())
			nt.Close()
			os.Exit(1)
		})
	}
	return nil
}
