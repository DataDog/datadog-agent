// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kprobe

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const probeUID = "net"

type TracerType int

const (
	TracerTypePrebuilt TracerType = iota
	TracerTypeRuntimeCompiled
	TracerTypeCORE
)

var (
	// The kernel has to be newer than 4.7.0 since we are using bpf_skb_load_bytes (4.5.0+) method to read from the
	// socket filter, and a tracepoint (4.7.0+).
	classificationMinimumKernel = kernel.VersionCode(4, 7, 0)

	protocolClassificationTailCalls = []manager.TailCallRoute{
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationQueues,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierQueuesSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationDBs,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierDBsSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationGRPC,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierGRPCSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.TCPCloseProgsMap,
			Key:           0,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.TCPCloseFlushReturn,
				UID:          probeUID,
			},
		},
	}

	// these primarily exist for mocking out in tests
	coreTracerLoader          = loadCORETracer
	rcTracerLoader            = loadRuntimeCompiledTracer
	prebuiltTracerLoader      = loadPrebuiltTracer
	tracerOffsetGuesserRunner = offsetguess.TracerOffsets.Offsets

	errCORETracerNotSupported = errors.New("CO-RE tracer not supported on this platform")
)

// ClassificationSupported returns true if the current kernel version supports the classification feature.
// The kernel has to be newer than 4.7.0 since we are using bpf_skb_load_bytes (4.5.0+) method to read from the socket
// filter, and a tracepoint (4.7.0+)
func ClassificationSupported(config *config.Config) bool {
	if !config.ProtocolClassificationEnabled {
		return false
	}
	if !config.CollectTCPv4Conns && !config.CollectTCPv6Conns {
		return false
	}
	currentKernelVersion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. classification monitoring disabled.")
		return false
	}

	return currentKernelVersion >= classificationMinimumKernel
}

func addBoolConst(options *manager.Options, flag bool, name string) {
	val := uint64(1)
	if !flag {
		val = uint64(0)
	}

	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{
			Name:  name,
			Value: val,
		},
	)
}

// LoadTracer loads the co-re/prebuilt/runtime compiled network tracer, depending on config
func LoadTracer(cfg *config.Config, mgrOpts manager.Options, perfHandlerTCP *ddebpf.PerfHandler) (*manager.Manager, func(), TracerType, error) {
	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}

	mgrOpts.DefaultKprobeAttachMethod = kprobeAttachMethod

	if cfg.EnableCORE {
		err := isCORETracerSupported()
		if err != nil && !errors.Is(err, errCORETracerNotSupported) {
			return nil, nil, TracerTypeCORE, fmt.Errorf("error determining if CO-RE tracer is supported: %w", err)
		}

		var m *manager.Manager
		var closeFn func()
		if err == nil {
			m, closeFn, err = coreTracerLoader(cfg, mgrOpts, perfHandlerTCP)
			// if it is a verifier error, bail always regardless of
			// whether a fallback is enabled in config
			var ve *ebpf.VerifierError
			if err == nil || errors.As(err, &ve) {
				return m, closeFn, TracerTypeCORE, err
			}
			// do not use offset guessing constants with runtime compilation
			mgrOpts.ConstantEditors = nil
		}

		if cfg.EnableRuntimeCompiler && cfg.AllowRuntimeCompiledFallback {
			log.Warnf("error loading CO-RE network tracer, falling back to runtime compiled: %s", err)
		} else if cfg.AllowPrecompiledFallback {
			log.Warnf("error loading CO-RE network tracer, falling back to pre-compiled: %s", err)
		} else {
			return nil, nil, TracerTypeCORE, fmt.Errorf("error loading CO-RE network tracer: %w", err)
		}
	}

	if cfg.EnableRuntimeCompiler && (!cfg.EnableCORE || cfg.AllowRuntimeCompiledFallback) {
		m, closeFn, err := rcTracerLoader(cfg, mgrOpts, perfHandlerTCP)
		if err == nil {
			return m, closeFn, TracerTypeRuntimeCompiled, err
		}

		if !cfg.AllowPrecompiledFallback {
			return nil, nil, TracerTypeRuntimeCompiled, fmt.Errorf("error compiling network tracer: %w", err)
		}

		log.Warnf("error compiling network tracer, falling back to pre-compiled: %s", err)
	}

	offsets, err := tracerOffsetGuesserRunner(cfg)
	if err != nil {
		return nil, nil, TracerTypePrebuilt, fmt.Errorf("error loading prebuilt tracer: error guessing offsets: %s", err)
	}

	mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, offsets...)

	m, closeFn, err := prebuiltTracerLoader(cfg, mgrOpts, perfHandlerTCP)
	return m, closeFn, TracerTypePrebuilt, err
}

func loadTracerFromAsset(buf bytecode.AssetReader, runtimeTracer, coreTracer bool, config *config.Config, mgrOpts manager.Options, perfHandlerTCP *ddebpf.PerfHandler) (*manager.Manager, func(), error) {
	m := &manager.Manager{}
	if err := initManager(m, config, perfHandlerTCP, runtimeTracer); err != nil {
		return nil, nil, fmt.Errorf("could not initialize manager: %w", err)
	}

	telemetryMapKeys := errtelemetry.BuildTelemetryKeys(m)
	mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, telemetryMapKeys...)

	var undefinedProbes []manager.ProbeIdentificationPair

	var closeProtocolClassifierSocketFilterFn func()
	classificationSupported := ClassificationSupported(config)
	addBoolConst(&mgrOpts, classificationSupported, "protocol_classification_enabled")

	if classificationSupported {
		socketFilterProbe, _ := m.GetProbe(manager.ProbeIdentificationPair{
			EBPFFuncName: probes.ProtocolClassifierEntrySocketFilter,
			UID:          probeUID,
		})
		if socketFilterProbe == nil {
			return nil, nil, fmt.Errorf("error retrieving protocol classifier socket filter")
		}

		var err error
		closeProtocolClassifierSocketFilterFn, err = filter.HeadlessSocketFilter(config, socketFilterProbe)
		if err != nil {
			return nil, nil, fmt.Errorf("error enabling protocol classifier: %w", err)
		}

		undefinedProbes = append(undefinedProbes, protocolClassificationTailCalls[0].ProbeIdentificationPair)
		mgrOpts.TailCallRouter = append(mgrOpts.TailCallRouter, protocolClassificationTailCalls...)
	} else {
		// Kernels < 4.7.0 do not know about the per-cpu array map used
		// in classification, preventing the program to load even though
		// we won't use it. We change the type to a simple array map to
		// circumvent that.
		for _, mapName := range []string{probes.ProtocolClassificationBufMap, probes.KafkaClientIDBufMap, probes.KafkaTopicNameBufMap} {
			mgrOpts.MapSpecEditors[mapName] = manager.MapSpecEditor{
				Type:       ebpf.Array,
				EditorFlag: manager.EditType,
			}
		}
	}

	if err := errtelemetry.ActivateBPFTelemetry(m, undefinedProbes); err != nil {
		return nil, nil, fmt.Errorf("could not activate ebpf telemetry: %w", err)
	}

	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledProbes(config, runtimeTracer, coreTracer)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[p.EBPFFuncName]; !enabled {
			mgrOpts.ExcludedFunctions = append(mgrOpts.ExcludedFunctions, p.EBPFFuncName)
		}
	}

	var tailCallsIdentifiersSet map[manager.ProbeIdentificationPair]struct{}
	if classificationSupported {
		tailCallsIdentifiersSet = make(map[manager.ProbeIdentificationPair]struct{}, len(protocolClassificationTailCalls))
		for _, tailCall := range protocolClassificationTailCalls {
			tailCallsIdentifiersSet[tailCall.ProbeIdentificationPair] = struct{}{}
		}
	}

	for funcName := range enabledProbes {
		probeIdentifier := manager.ProbeIdentificationPair{
			EBPFFuncName: funcName,
			UID:          probeUID,
		}
		if _, ok := tailCallsIdentifiersSet[probeIdentifier]; ok {
			// tail calls should be enabled (a.k.a. not excluded) but not activated.
			continue
		}
		mgrOpts.ActivatedProbes = append(
			mgrOpts.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: probeIdentifier,
			})
	}

	if err := m.InitWithOptions(buf, mgrOpts); err != nil {
		return nil, nil, fmt.Errorf("failed to init ebpf manager: %w", err)
	}

	return m, closeProtocolClassifierSocketFilterFn, nil
}

func loadCORETracer(config *config.Config, mgrOpts manager.Options, perfHandlerTCP *ddebpf.PerfHandler) (*manager.Manager, func(), error) {
	var m *manager.Manager
	var closeFn func()
	var err error
	err = ddebpf.LoadCOREAsset(netebpf.ModuleFileName("tracer", config.BPFDebug), func(ar bytecode.AssetReader, o manager.Options) error {
		o.RLimit = mgrOpts.RLimit
		o.MapSpecEditors = mgrOpts.MapSpecEditors
		o.ConstantEditors = mgrOpts.ConstantEditors
		o.DefaultKprobeAttachMethod = mgrOpts.DefaultKprobeAttachMethod
		m, closeFn, err = loadTracerFromAsset(ar, false, true, config, o, perfHandlerTCP)
		return err
	})

	return m, closeFn, err
}

func loadRuntimeCompiledTracer(config *config.Config, mgrOpts manager.Options, perfHandlerTCP *ddebpf.PerfHandler) (*manager.Manager, func(), error) {
	buf, err := getRuntimeCompiledTracer(config)
	if err != nil {
		return nil, nil, err
	}
	defer buf.Close()

	return loadTracerFromAsset(buf, true, false, config, mgrOpts, perfHandlerTCP)
}

func loadPrebuiltTracer(config *config.Config, mgrOpts manager.Options, perfHandlerTCP *ddebpf.PerfHandler) (*manager.Manager, func(), error) {
	buf, err := netebpf.ReadBPFModule(config.BPFDir, config.BPFDebug)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read bpf module: %w", err)
	}
	defer buf.Close()

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, nil, fmt.Errorf("kernel version: %s", err)
	}
	// prebuilt on 5.18+ cannot support UDPv6
	if kv >= kernel.VersionCode(5, 18, 0) {
		config.CollectUDPv6Conns = false
	}

	return loadTracerFromAsset(buf, false, false, config, mgrOpts, perfHandlerTCP)
}

func isCORETracerSupported() error {
	kv, err := kernel.HostVersion()
	if err != nil {
		return err
	}
	if kv >= kernel.VersionCode(4, 4, 128) {
		return nil
	}

	platform, err := kernel.Platform()
	if err != nil {
		return err
	}

	// centos/redhat distributions we support
	// can have kernel versions < 4, and
	// CO-RE is supported there
	if platform == "centos" || platform == "redhat" {
		return nil
	}

	return errCORETracerNotSupported
}
