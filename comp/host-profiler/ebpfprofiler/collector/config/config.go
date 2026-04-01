// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/linux"
	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/tracer"
)

const (
	// 1TB of executable address space
	MaxArgMapScaleFactor = 8
)

// Config is the configuration for the collector.
type Config struct {
	ReporterInterval       time.Duration `mapstructure:"reporter_interval"`
	ReporterJitter         float64       `mapstructure:"reporter_jitter"`
	MonitorInterval        time.Duration `mapstructure:"monitor_interval"`
	SamplesPerSecond       int           `mapstructure:"samples_per_second"`
	ProbabilisticInterval  time.Duration `mapstructure:"probabilistic_interval"`
	ProbabilisticThreshold uint          `mapstructure:"probabilistic_threshold"`
	Tracers                string        `mapstructure:"tracers"`
	ClockSyncInterval      time.Duration `mapstructure:"clock_sync_interval"`
	SendErrorFrames        bool          `mapstructure:"send_error_frames"`
	SendIdleFrames         bool          `mapstructure:"send_idle_frames"`
	VerboseMode            bool          `mapstructure:"verbose_mode"`
	OffCPUThreshold        float64       `mapstructure:"off_cpu_threshold"`
	IncludeEnvVars         string        `mapstructure:"include_env_vars"`
	ProbeLinks             []string      `mapstructure:"probe_links"`
	LoadProbe              bool          `mapstructure:"load_probe"`
	MapScaleFactor         uint          `mapstructure:"map_scale_factor"`
	BPFVerifierLogLevel    uint          `mapstructure:"bpf_verifier_log_level"`
	NoKernelVersionCheck   bool          `mapstructure:"no_kernel_version_check"`
	MaxGRPCRetries         uint32        `mapstructure:"max_grpc_retries"`
	MaxRPCMsgSize          int           `mapstructure:"max_rpc_msg_size"`
}

// Validate validates the config.
// This is automatically called by the config parser as it implements the xconfmap.Validator interface.
func (cfg *Config) Validate() error {
	if cfg.SamplesPerSecond < 1 {
		return fmt.Errorf("invalid sampling frequency: %d", cfg.SamplesPerSecond)
	}

	if cfg.MapScaleFactor > MaxArgMapScaleFactor {
		return fmt.Errorf(
			"eBPF map scaling factor %d exceeds limit (max: %d)",
			cfg.MapScaleFactor, MaxArgMapScaleFactor,
		)
	}

	if cfg.BPFVerifierLogLevel > 2 {
		return fmt.Errorf("invalid eBPF verifier log level: %d", cfg.BPFVerifierLogLevel)
	}

	if cfg.ProbabilisticInterval < 1*time.Minute || cfg.ProbabilisticInterval > 5*time.Minute {
		return errors.New(
			"invalid argument for probabilistic-interval: use " +
				"a duration between 1 and 5 minutes",
		)
	}

	if cfg.ProbabilisticThreshold < 1 ||
		cfg.ProbabilisticThreshold > tracer.ProbabilisticThresholdMax {
		return fmt.Errorf(
			"invalid argument for probabilistic-threshold. Value "+
				"should be between 1 and %d",
			tracer.ProbabilisticThresholdMax,
		)
	}

	if cfg.OffCPUThreshold < 0.0 || cfg.OffCPUThreshold > 1.0 {
		return errors.New(
			"invalid argument for off-cpu-threshold. The value " +
				"should be in the range [0..1]. 0 disables off-cpu profiling")
	}

	if cfg.ReporterJitter < 0.0 || cfg.ReporterJitter > 1.0 {
		return errors.New(
			"invalid argument for reporter-jitter. The value " +
				"should be in the range [0..1]. 0 disables jitter")
	}

	if !cfg.NoKernelVersionCheck {
		major, minor, patch, err := linux.GetCurrentKernelVersion()
		if err != nil {
			return fmt.Errorf("failed to get kernel version: %v", err)
		}

		var minMajor, minMinor uint32
		switch runtime.GOARCH {
		case "amd64":
			minMajor, minMinor = 5, 2
		case "arm64":
			// Older ARM64 kernel versions have broken bpf_probe_read.
			// https://github.com/torvalds/linux/commit/6ae08ae3dea2cfa03dd3665a3c8475c2d429ef47
			minMajor, minMinor = 5, 5
		default:
			return fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}

		if major < minMajor || (major == minMajor && minor < minMinor) {
			return fmt.Errorf("host Agent requires kernel version "+
				"%d.%d or newer but got %d.%d.%d", minMajor, minMinor, major, minor, patch)
		}
	}

	return nil
}
