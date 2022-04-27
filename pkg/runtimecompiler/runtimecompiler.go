// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

//go:generate go run ../ebpf/include_headers.go ../network/ebpf/c/runtime/tracer.c ../ebpf/bytecode/build/runtime/tracer.c ../ebpf/c ../network/ebpf/c/runtime ../network/ebpf/c
//go:generate go run ../ebpf/bytecode/runtime/integrity.go ../ebpf/bytecode/build/runtime/tracer.c ../ebpf/bytecode/runtime/tracer.go runtime

//go:generate go run ../ebpf/include_headers.go ../network/ebpf/c/runtime/conntrack.c ../ebpf/bytecode/build/runtime/conntrack.c ../ebpf/c ../network/ebpf/c/runtime ../network/ebpf/c
//go:generate go run ../ebpf/bytecode/runtime/integrity.go ../ebpf/bytecode/build/runtime/conntrack.c ../ebpf/bytecode/runtime/conntrack.go runtime

//go:generate go run ../ebpf/include_headers.go ../network/ebpf/c/runtime/http.c ../ebpf/bytecode/build/runtime/http.c ../ebpf/c ../network/ebpf/c/runtime ../network/ebpf/c
//go:generate go run ../ebpf/bytecode/runtime/integrity.go ../ebpf/bytecode/build/runtime/http.c ../ebpf/bytecode/runtime/http.go runtime

// TODO change probe.c path to runtime-compilation specific version
//go:generate go run ../ebpf/include_headers.go ../security/ebpf/c/prebuilt/probe.c ../ebpf/bytecode/build/runtime/runtime-security.c ../security/ebpf/c ../ebpf/c
//go:generate go run ../ebpf/bytecode/runtime/integrity.go ../ebpf/bytecode/build/runtime/runtime-security.c ../ebpf/bytecode/runtime/runtime-security.go runtime

//go:generate go run ../ebpf/include_headers.go ../collector/corechecks/ebpf/c/runtime/tcp-queue-length-kern.c ../ebpf/bytecode/build/runtime/tcp-queue-length.c ../ebpf/c
//go:generate go run ../ebpf/bytecode/runtime/integrity.go ../ebpf/bytecode/build/runtime/tcp-queue-length.c ../ebpf/bytecode/runtime/tcp-queue-length.go runtime

//go:generate go run ../ebpf/include_headers.go ../collector/corechecks/ebpf/c/runtime/oom-kill-kern.c ../ebpf/bytecode/build/runtime/oom-kill.c ../ebpf/c
//go:generate go run ../ebpf/bytecode/runtime/integrity.go ..//ebpf/bytecode/build/runtime/oom-kill.c ../ebpf/bytecode/runtime/oom-kill.go runtime

package runtimecompiler

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/runtimecompiler/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	seckernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// RuntimeCompiler is responsible for runtime compiling eBPF programs across the system-probe.
// Runtime compilation must be performed prior to any other modules being started in order
// for those modules to be able to correctly load runtime compiled programs.
var RuntimeCompiler = newRuntimeCompiler()

const metricPrefix = "datadog.system_probe.runtime_compiler"

type runtimeCompiler struct {
	enabled      bool
	config       *config.Config
	statsdClient statsd.ClientInterface

	headerFetchResult kernel.HeaderFetchResult
	telemetryByAsset  map[string]runtime.RuntimeCompilationTelemetry
}

func newRuntimeCompiler() runtimeCompiler {
	return runtimeCompiler{
		enabled:           false,
		config:            nil,
		statsdClient:      nil,
		headerFetchResult: kernel.NotAttempted,
		telemetryByAsset:  make(map[string]runtime.RuntimeCompilationTelemetry),
	}
}

// Init initializes the runtime compiler. This should be called prior to calling Run
func (rc *runtimeCompiler) Init(cfg *config.Config, statsdClient statsd.ClientInterface) {
	compilationRequired := cfg.EnableNetworkCompilation ||
		cfg.EnableRuntimeSecurityCompilation ||
		cfg.EnableConstantFetcherCompilation ||
		cfg.EnableTcpQueueLengthCompilation ||
		cfg.EnableOomKillCompilation

	rc.enabled = compilationRequired
	rc.config = cfg
	rc.statsdClient = statsdClient
}

func (rc *runtimeCompiler) Run() error {
	if !rc.enabled {
		log.Debugf("runtime compilation not enabled")
		return nil
	}

	defer rc.sendMetrics()

	kernelHeaders, res, err := kernel.GetKernelHeaders(
		rc.config.EnableKernelHeaderDownload,
		rc.config.KernelHeadersDirs,
		rc.config.KernelHeadersDownloadDir,
		rc.config.AptConfigDir,
		rc.config.YumReposDir,
		rc.config.ZypperReposDir)
	rc.headerFetchResult = res
	if err != nil {
		return fmt.Errorf("unable to find kernel headers: %w", err)
	}

	if rc.config.EnableNetworkCompilation {
		rc.compileNetworkAssets(kernelHeaders)
	}

	if rc.config.EnableRuntimeSecurityCompilation {
		rc.compileRuntimeSecurityAssets(kernelHeaders)
	}

	if rc.config.EnableConstantFetcherCompilation {
		rc.compileConstantFetcher(kernelHeaders)
	}

	if rc.config.EnableTcpQueueLengthCompilation {
		rc.compileTcpQueueLengthProbe(kernelHeaders)
	}

	if rc.config.EnableOomKillCompilation {
		rc.compileOomKillProbe(kernelHeaders)
	}

	return nil
}

func (rc *runtimeCompiler) compileNetworkAssets(kernelHeaders []string) {
	cflags := runtime.GetNetworkAssetCFlags(rc.config.CollectIPv6Conns, rc.config.BPFDebugEnabled)
	bpfDir := rc.config.BPFDir
	outputDir := rc.config.RuntimeCompiledAssetDir

	telemetry, err := runtime.Tracer.Compile(bpfDir, outputDir, cflags, kernelHeaders)
	if err != nil {
		log.Errorf("error compiling network tracer: %s", err)
	}
	rc.telemetryByAsset["network_tracer"] = telemetry

	if rc.config.ConntrackEnabled {
		telemetry, err = runtime.Conntrack.Compile(bpfDir, outputDir, cflags, kernelHeaders)
		if err != nil {
			log.Errorf("error compiling ebpf conntracker: %s", err)
		}
		rc.telemetryByAsset["conntrack"] = telemetry
	}

	if rc.config.HTTPMonitoringEnabled {
		telemetry, err = runtime.Http.Compile(bpfDir, outputDir, cflags, kernelHeaders)
		if err != nil {
			log.Errorf("error compiling network http tracer: %s", err)
		}
		rc.telemetryByAsset["http_tracer"] = telemetry
	}
}

func (rc *runtimeCompiler) compileRuntimeSecurityAssets(kernelHeaders []string) {
	useSyscallWrapper, err := ebpf.IsSyscallWrapperRequired()
	if err != nil {
		log.Errorf("unable to compile runtime security assets: error checking if syscall wrapper is required: %s", err)
		return
	}

	cflags := runtime.GetSecurityAssetCFlags(useSyscallWrapper)
	bpfDir := rc.config.BPFDir
	outputDir := rc.config.RuntimeCompiledAssetDir

	telemetry, err := runtime.RuntimeSecurity.Compile(bpfDir, outputDir, cflags, kernelHeaders)
	if err != nil {
		log.Errorf("error compiling runtime-security probe: %s", err)
	}
	rc.telemetryByAsset["runtime_security_probe"] = telemetry
}

func (rc *runtimeCompiler) compileConstantFetcher(kernelHeaders []string) {
	kernelVersion, err := seckernel.NewKernelVersion()
	if err != nil {
		log.Errorf("unable to compile constant fetcher: unable to detect the kernel version: %s", err)
		return
	}

	constantFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&rc.config.Config)
	probe.AppendProbeRequestsToFetcher(constantFetcher, kernelVersion)
	cCode, err := constantFetcher.GetCCode()
	if err != nil {
		log.Errorf("unable to compile constant fetcher: error generating c code: %s", err)
		return
	}

	telemetry, err := runtime.ConstantFetcher.Compile(rc.config.RuntimeCompiledAssetDir, cCode, nil, kernelHeaders)
	if err != nil {
		log.Errorf("unable to compile constant fetcher: %s", err)
	}
	rc.telemetryByAsset["constant_fetcher"] = telemetry
}

func (rc *runtimeCompiler) compileTcpQueueLengthProbe(kernelHeaders []string) {
	telemetry, err := runtime.TcpQueueLength.Compile(rc.config.BPFDir, rc.config.RuntimeCompiledAssetDir, nil, kernelHeaders)
	if err != nil {
		log.Errorf("error compiling tcp queue length probe: %s", err)
	}
	rc.telemetryByAsset["tcp_queue_length"] = telemetry
}

func (rc *runtimeCompiler) compileOomKillProbe(kernelHeaders []string) {
	telemetry, err := runtime.OomKill.Compile(rc.config.BPFDir, rc.config.RuntimeCompiledAssetDir, nil, kernelHeaders)
	if err != nil {
		log.Errorf("error compiling oom kill probe: %s", err)
	}
	rc.telemetryByAsset["oom_kill"] = telemetry
}

func (rc *runtimeCompiler) sendMetrics() {
	if rc.statsdClient != nil {
		log.Infof("sending runtime compilation telemetry to statsd")

		tags := []string{fmt.Sprintf("version:%s", version.AgentVersion)}

		rc.statsdClient.Gauge(metricPrefix+".header_fetch_result", float64(rc.headerFetchResult), tags, 1)

		for assetName, tm := range rc.telemetryByAsset {
			tm.SendMetrics(metricPrefix+"."+assetName, rc.statsdClient, tags)
		}
	}
}

func (rc *runtimeCompiler) GetHeaderFetchTelemetry() kernel.HeaderFetchResult {
	return rc.headerFetchResult
}

func (rc *runtimeCompiler) GetCompilationTelemetry() map[string]runtime.RuntimeCompilationTelemetry {
	return rc.telemetryByAsset
}
