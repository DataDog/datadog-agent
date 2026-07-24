// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package systemprobeimpl implements the remoteagent component interface
package systemprobeimpl

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/compliance/statusregistry"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/security/flareregistry"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Requires defines the dependencies for the remoteagent component
type Requires struct {
	Lifecycle      compdef.Lifecycle
	Log            log.Component
	IPC            ipc.Component
	Config         config.Component
	SysProbeConfig sysprobeconfig.Component
	Telemetry      telemetry.Component
}

// Provides defines the output of the remoteagent component
type Provides struct {
	Comp remoteagent.Component
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) (Provides, error) {
	// Check if the remoteAgentRegistry is enabled
	if !reqs.Config.GetBool("remote_agent.registry.enabled") {
		return Provides{}, nil
	}

	// Get the registry address
	registryAddress := net.JoinHostPort(reqs.Config.GetString("cmd_host"), reqs.Config.GetString("cmd_port"))

	remoteAgentServer, err := helper.NewUnimplementedRemoteAgentServer(reqs.IPC, reqs.Log, reqs.Config, reqs.Lifecycle, registryAddress, flavor.GetFlavor(), flavor.GetHumanReadableFlavor())
	if err != nil {
		return Provides{}, err
	}

	// Set the agent identity for log metrics partitioning so that
	// logs.bytes_sent is tagged with emitter="system-probe".
	metrics.SetAgentIdentity("system-probe")

	remoteagentImpl := &remoteagentImpl{
		log:               reqs.Log,
		ipc:               reqs.IPC,
		cfg:               reqs.Config,
		sysProbeConfig:    reqs.SysProbeConfig,
		telemetry:         reqs.Telemetry,
		remoteAgentServer: remoteAgentServer,
	}

	// Add your gRPC services implementations here:
	pbcore.RegisterTelemetryProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)
	pbcore.RegisterStatusProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)
	pbcore.RegisterFlareProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)

	remoteAgentServer.Start()

	provides := Provides{
		Comp: remoteagentImpl,
	}
	return provides, nil
}

type remoteagentImpl struct {
	log            log.Component
	ipc            ipc.Component
	cfg            config.Component
	sysProbeConfig sysprobeconfig.Component
	telemetry      telemetry.Component

	remoteAgentServer *helper.UnimplementedRemoteAgentServer
	pbcore.UnimplementedTelemetryProviderServer
	pbcore.UnimplementedStatusProviderServer
	pbcore.UnimplementedFlareProviderServer
}

func (r *remoteagentImpl) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	text, registered, err := statusregistry.GetTextOrError()
	if !registered {
		return &pbcore.GetStatusDetailsResponse{}, nil
	}
	if err != nil {
		return &pbcore.GetStatusDetailsResponse{}, nil
	}
	return &pbcore.GetStatusDetailsResponse{
		NamedSections: map[string]*pbcore.StatusSection{
			"Compliance": {
				Fields: map[string]string{
					"": text,
				},
			},
		},
	}, nil
}

// WaitSessionID blocks until the remote agent is registered and a session ID is available.
// This allows components that need the session ID (e.g. config stream consumer) to wait for RAR registration.
func (r *remoteagentImpl) WaitSessionID(ctx context.Context) (string, error) {
	return r.remoteAgentServer.WaitSessionID(ctx)
}

// GetFlareFiles collects flare data that lives in system-probe and ships it back
// to the Core Agent via the remote-agent registry. Each section is gated on its
// feature flag so disabled features don't bloat the flare.
func (r *remoteagentImpl) GetFlareFiles(_ context.Context, _ *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	files := map[string][]byte{}

	r.log.Warnf("[debug] runtime_security_config.enabled value=%v source=%v file=%q",
		r.sysProbeConfig.GetBool("runtime_security_config.enabled"),
		r.sysProbeConfig.GetSource("runtime_security_config.enabled"),
		r.sysProbeConfig.ConfigFileUsed())

	if r.sysProbeConfig.GetBool("runtime_security_config.enabled") {
		if policies, registered, err := flareregistry.GetLoadedPolicies(true); err != nil {
			r.log.Warnf("failed to get loaded runtime-security policies for flare: %v", err)
		} else if registered {
			files["runtime-security-policies.json"] = policies
		}

		collectRuntimeSecurityKernelArtifacts(r.log, files)
	}

	if r.cfg.GetBool("compliance_config.enabled") {
		collectComplianceFiles(r.log, r.cfg.GetString("compliance_config.dir"), files)
	}

	return &pbcore.GetFlareFilesResponse{Files: files}, nil
}

// collectComplianceFiles walks complianceDir and adds every regular file (no
// symlinks, to avoid leaking sensitive linked content) into files under a
// "compliance.d/<basename>" key.
func collectComplianceFiles(logger log.Component, complianceDir string, files map[string][]byte) {
	if complianceDir == "" {
		return
	}
	entries, err := os.ReadDir(complianceDir)
	if err != nil {
		logger.Warnf("failed to list compliance directory %q for flare: %v", complianceDir, err)
		return
	}
	for _, entry := range entries {
		path := filepath.Join(complianceDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			logger.Warnf("failed to read compliance file %q for flare: %v", path, err)
			continue
		}
		files["compliance.d/"+entry.Name()] = content
	}
}

func (r *remoteagentImpl) GetTelemetry(_ context.Context, _ *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	prometheusText, err := r.telemetry.GatherText(false, telemetry.StaticMetricFilter(
		// Metrics to forward from system-probe to core agent.
		// The emitter tag is set to "system-probe" via metrics.SetAgentIdentity() above.
		"logs__bytes_sent",
		"logs__encoded_bytes_sent",

		// Windows Injector metrics (using double underscore format from telemetry component)
		"injector__processes_added_to_injection_tracker",
		"injector__processes_removed_from_injection_tracker",
		"injector__processes_skipped_subsystem",
		"injector__processes_skipped_container",
		"injector__processes_skipped_protected",
		"injector__processes_skipped_system",
		"injector__processes_skipped_excluded",
		"injector__injection_attempts",
		"injector__injection_attempt_failures",
		"injector__injection_max_time_us",
		"injector__injection_successes",
		"injector__injection_failures",
		"injector__pe_caching_failures",
		"injector__import_directory_restoration_failures",
		"injector__pe_memory_allocation_failures",
		"injector__pe_injection_context_allocated",
		"injector__pe_injection_context_cleanedup",
		"injector__crashes_during_injection",
		"injector__crashes_post_injection",
		"injector__boot_recovery_crash_boots_detected",
		"injector__boot_recovery_driver_self_disabled",
		"injector__boot_recovery_stability_timer_fired",

		// eBPF metrics
		"ebpf__core_load_success",
		"ebpf__core_load_error",
		"ebpf__core_remoteconfig_success",
		"ebpf__core_remoteconfig_error",
	))
	if err != nil {
		return nil, err
	}

	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: prometheusText,
		},
	}, nil
}
