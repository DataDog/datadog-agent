// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"context"
	"maps"
	"os"
	"runtime"
	"strings"
	"time"

	"log"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/lifecycle"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

const (
	// MicroVM's resource_type tag value
	MicroVMResourceType = "lambdamicrovm"

	// MicroVMOrigin origin tag value
	MicroVMOrigin = MicroVMResourceType

	// MicroVM resource_provider tag value
	MicroVMResourceProvider = "aws"

	// Microvm metric prefix
	MicroVMPrefix = "aws.lambda.microvm."

	// MicroVm usage metric name suffix
	MicroVMUsageMetricSuffix = "instance"
)

// LifecycleContext carries the telemetry dependencies needed by MicroVM.Init to
// construct and start the lifecycle hook server. Populated by main.go before
// calling CloudService.Init; nil (and ignored) for all non-MicroVM services.
type LifecycleContext struct {
	MetricFlusher  lifecycle.Flusher
	LogsFlusher    lifecycle.LogsFlusher
	MetricEmitter  lifecycle.MetricEmitter
	SampleDrainer  lifecycle.SampleDrainer
	FlushTimeout   time.Duration
	SidecarMode    bool
	LogsTagSetter  lifecycle.LogsTagSetter  // nil-safe; applied via server.SetLogsTagSetter after /run
	BaseTags       []string                 // startup log tag snapshot passed alongside LogsTagSetter
	TraceTagSetter lifecycle.TraceTagSetter // nil-safe; applied via server.SetTraceTagSetter after /run
	BaseTraceTags  map[string]string        // startup trace tag snapshot passed alongside TraceTagSetter
}

// MicroVM implements CloudService for AWS Lambda MicroVMs.
type MicroVM struct {
	server       *lifecycle.Server
	child        *lifecycle.Child
	flushTimeout time.Duration
}

// GetTags returns MicroVM-specific tags parsed from the image ARN env var.
func (m *MicroVM) GetTags() map[string]string {
	tags := map[string]string{
		"origin":            MicroVMOrigin,
		"_dd.origin":        MicroVMOrigin,
		"resource_type":     MicroVMResourceType,
		"resource_provider": MicroVMResourceProvider,
	}

	arn := os.Getenv(serverlessenv.MicroVMImageARNEnvVar)
	if arn == "" {
		tags["region"] = "unknown"
		tags["account_id"] = "unknown"
		tags["image_name"] = "unknown"
		tags["resource_id"] = "unknown"
		return tags
	}

	region, accountID, imageName := parseMicroVMARN(arn)
	tags["region"] = region
	tags["account_id"] = accountID
	tags["image_name"] = imageName
	tags["resource_id"] = arn

	return tags
}

// GetEnhancedMetricTags returns base (low-cardinality) and usage tags.
// instance_id is absent from Usage tags at startup because the MicroVM ID is
// not known until the /run lifecycle hook fires.
func (m *MicroVM) GetEnhancedMetricTags(tags map[string]string) EnhancedMetricTags {
	baseTags := map[string]string{
		"account_id":        tagValueOrUnknown(tags["account_id"]),
		"image_name":        tagValueOrUnknown(tags["image_name"]),
		"origin":            tagValueOrUnknown(tags["origin"]),
		"region":            tagValueOrUnknown(tags["region"]),
		"resource_type":     tagValueOrUnknown(tags["resource_type"]),
		"resource_provider": tagValueOrUnknown(tags["resource_provider"]),
		"resource_id":       tagValueOrUnknown(tags["resource_id"]),
	}
	return EnhancedMetricTags{Base: baseTags, Usage: maps.Clone(baseTags)}
}

// GetDefaultLogsSource returns the default logs source.
func (m *MicroVM) GetDefaultLogsSource() string { return MicroVMOrigin }

// GetMetricPrefix returns the AWS MicroVM metric prefix.
func (m *MicroVM) GetMetricPrefix() string { return MicroVMPrefix }

// GetUsageMetricSuffix returns the usage metric suffix.
func (m *MicroVM) GetUsageMetricSuffix() string { return MicroVMUsageMetricSuffix }

// GetOrigin returns the origin tag value.
func (m *MicroVM) GetOrigin() string { return MicroVMOrigin }

// GetSource returns the metrics source.
func (m *MicroVM) GetSource() metrics.MetricSource {
	return metrics.MetricSourceAWSMicroVMEnhanced
}

// isSupportedArch reports whether arch is supported by MicroVM.
// MicroVM supports both amd64 and arm64; all other cloud services are amd64-only.
func isSupportedArch(arch string) bool {
	return arch == archAMD64 || arch == archARM64
}

// Init starts the MicroVM lifecycle hook server.
func (m *MicroVM) Init(ctx *TracingContext) error {
	if arch := runtime.GOARCH; !isSupportedArch(arch) {
		log.Fatalf(unsupportedArchMsg, arch)
	}
	if ctx == nil || ctx.LifecycleCtx == nil {
		return nil
	}
	lc := ctx.LifecycleCtx
	m.flushTimeout = lc.FlushTimeout

	components, err := lifecycle.SetupFromEnv(lc.SidecarMode)
	if err != nil {
		log.Printf("Invalid lifecycle env-var config (%v); starting with defaults", err)
		components = lifecycle.SetupFallback(lc.SidecarMode)
	}
	m.child = components.Child

	arn := os.Getenv(serverlessenv.MicroVMImageARNEnvVar)
	if arn == "" {
		arn = "unknown"
	}
	heartbeat := lifecycle.NewHeartbeat(
		lifecycle.DefaultHeartbeatInterval,
		lc.MetricEmitter,
		m.GetSource(),
		[]string{"microvm_image_arn:" + arn},
	)
	m.server = lifecycle.NewServer(
		components.Port,
		lc.MetricFlusher,
		ctx.TraceAgent, // satisfies lifecycle.Flusher via TraceAgent.Flush()
		lc.LogsFlusher,
		lc.MetricEmitter,
		lc.SampleDrainer,
		m.GetSource(),
		lc.FlushTimeout,
		components.Handle,
		components.Forwarder,
		heartbeat,
	)
	if lc.LogsTagSetter != nil {
		m.server.SetLogsTagSetter(lc.LogsTagSetter, lc.BaseTags)
	}
	if lc.TraceTagSetter != nil {
		m.server.SetTraceTagSetter(lc.TraceTagSetter, lc.BaseTraceTags)
	}
	l, err := m.server.Listen()
	if err != nil {
		log.Fatalf("MicroVM lifecycle server failed to bind: %v", err)
	}
	go m.server.Serve(l)
	return nil
}

// Child returns the *lifecycle.Child that mode.RunInit uses for /ready
// alive-checking. Nil in sidecar mode or when Init has not been called.
func (m *MicroVM) Child() *lifecycle.Child { return m.child }

// Run spawns the user process in init-container mode. ProcessHooks bind
// m.child.MarkAlive/MarkDead so the lifecycle server's /ready alive-check
// reflects the user app's state without exposing *lifecycle.Child to the
// mode package. MicroVM is exclusively an init-container deployment; sidecar
// mode is a wiring error and is treated as fatal.
func (m *MicroVM) Run(modeConf mode.Conf, logConfig *serverlessInitLog.Config) error {
	if modeConf.SidecarMode {
		log.Fatalf("MicroVM does not support sidecar mode")
	}
	return mode.RunInit(logConfig, &mode.ProcessHooks{
		OnProcess: m.child.StoreProcess,
		OnAlive:   m.child.MarkAlive,
		OnDead:    m.child.MarkDead,
	})
}

// Shutdown stops the MicroVM lifecycle hook server so that any in-flight
// /suspend or /terminate request can complete before the metric and trace
// agents are torn down.
func (m *MicroVM) Shutdown(_ *serverlessMetrics.ServerlessMetricAgent, _ bool, _ error) {
	if m.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.flushTimeout)
	defer cancel()
	if err := m.server.Stop(ctx); err != nil {
		log.Printf("MicroVM lifecycle server shutdown error: %v", err)
	}
}

// AddStartMetric is a no-op for MicroVM. The lifecycle server emits the run
// metric when the /run hook fires; emitting it here would double-count.
func (m *MicroVM) AddStartMetric(_ *serverlessMetrics.ServerlessMetricAgent) {}

// ShouldForceFlushAllOnForceFlushToSerializer returns false for MicroVM.
func (m *MicroVM) ShouldForceFlushAllOnForceFlushToSerializer() bool { return false }

// isMicroVM returns true when running inside an AWS Lambda MicroVM.
func isMicroVM() bool {
	_, exists := os.LookupEnv(serverlessenv.MicroVMImageARNEnvVar)
	return exists
}

// parseMicroVMARN extracts region, accountID, and imageName from an ARN of the
// form arn:aws:lambda:<region>:<account>:microvm-image:<name>.
// Returns "unknown" for any field that cannot be parsed.
func parseMicroVMARN(arn string) (region, accountID, imageName string) {
	parts := strings.Split(arn, ":")
	region = "unknown"
	accountID = "unknown"
	imageName = "unknown"
	// ARN format: arn:aws:lambda:region:account:microvm-image:name
	if len(parts) >= 5 {
		if parts[3] != "" {
			region = parts[3]
		}
		if parts[4] != "" {
			accountID = parts[4]
		}
	}
	if len(parts) >= 7 && parts[6] != "" {
		imageName = strings.Join(parts[6:], ":")
	}
	return region, accountID, imageName
}
