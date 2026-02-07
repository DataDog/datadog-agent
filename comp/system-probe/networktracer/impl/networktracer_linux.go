// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package networktracerimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/sender"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	logutil "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the networktracer component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Telemetry      telemetry.Component
	Statsd         ddgostatsd.ClientInterface

	// direct send
	CoreConfig           config.Component
	Log                  log.Component
	WMeta                workloadmeta.Component
	Tagger               tagger.Component
	Hostname             hostname.Component
	ConnectionsForwarder connectionsforwarder.Component
	NPCollector          npcollector.Component
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

func (n *networkTracerComp) NeedsEBPF() bool {
	return tracer.NeedsEBPF()
}

func (n *networkTracerComp) OptionalEBPF() bool {
	return false
}

// NewComponent creates a new networktracer component
func NewComponent(reqs Requires) (Provides, error) {
	nt := &networkTracerComp{
		createFn: func() (types.SystemProbeModule, error) {
			return createNetworkTracerModule(
				reqs.Telemetry,
				reqs.Statsd,
				reqs.CoreConfig,
				reqs.Log,
				reqs.SysprobeConfig,
				reqs.Tagger,
				reqs.WMeta,
				reqs.Hostname,
				reqs.ConnectionsForwarder,
				reqs.NPCollector,
			)
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
	connsSender  sender.Sender
	ctx          context.Context
	cancelFunc   context.CancelFunc
}

// Close will stop all system probe activities
func (nt *networkTracer) Close() {
	if nt.connsSender != nil {
		nt.connsSender.Stop()
	}
	nt.tracer.Stop()
	nt.cancelFunc()
}

func createNetworkTracerModule(
	telemetry telemetry.Component,
	statsd ddgostatsd.ClientInterface,
	coreConfig config.Component,
	log log.Component,
	sysprobeconfig sysprobeconfig.Component,
	tagger tagger.Component,
	wmeta workloadmeta.Component,
	hostname hostname.Component,
	connectionsforwarder connectionsforwarder.Component,
	npcollector npcollector.Component,
) (types.SystemProbeModule, error) {
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

	ctx, cancel := context.WithCancel(context.Background())
	var connsSender sender.Sender
	if ncfg.DirectSend {
		connsSender, err = sender.New(ctx, t, sender.Dependencies{
			Config:         coreConfig,
			Logger:         log,
			Sysprobeconfig: sysprobeconfig,
			Tagger:         tagger,
			Wmeta:          wmeta,
			Hostname:       hostname,
			Forwarder:      connectionsforwarder,
			NPCollector:    npcollector,
		})
		if err != nil {
			t.Stop()
			cancel()
			return nil, fmt.Errorf("create direct sender: %s", err)
		}
	}

	return &networkTracer{
		tracer:      t,
		cfg:         ncfg,
		connsSender: connsSender,
		ctx:         ctx,
		cancelFunc:  cancel,
	}, nil
}

func (nt *networkTracer) platformRegister(httpMux types.SystemProbeRouter) error {
	httpMux.HandleFunc("/network_id", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		id, err := getNetworkID(req.Context())
		if err != nil {
			logutil.Debugf("unable to retrieve network ID: %s", err)
			w.WriteHeader(500)
			return
		}
		_, _ = io.WriteString(w, id)
	}))

	return nil
}

func getNetworkID(ctx context.Context) (string, error) {
	id := ""
	err := netnsutil.WithRootNS(kernel.ProcFSRoot(), func() error {
		var err error
		id, err = ec2.GetNetworkID(ctx)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}
