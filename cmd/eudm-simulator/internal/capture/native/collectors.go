// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows || darwin

package native

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	logimpl "github.com/DataDog/datadog-agent/comp/core/log/impl"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	filterimpl "github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/impl"
	hostimpl "github.com/DataDog/datadog-agent/comp/metadata/host/impl"
	gpusubscriberimpl "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/impl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/wlan"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/cpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type lifecycle struct{ hooks []compdef.Hook }

func (l *lifecycle) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }
func (l *lifecycle) stop() {
	for i := len(l.hooks) - 1; i >= 0; i-- {
		if l.hooks[i].OnStop != nil {
			_ = l.hooks[i].OnStop(context.Background())
		}
	}
}

type noResources struct{}

func (noResources) Get() map[string]interface{} { return nil }

type noEvents struct{}

func (noEvents) Get() (eventplatform.Forwarder, bool) { return nil, false }

func newCollectors(ctx context.Context, s *session, p *output.Pipeline) ([]scheduledCollector, func(), error) {
	logger := logimpl.NewTemporaryLoggerWithoutInit()
	actualHostname, err := os.Hostname()
	if err != nil {
		return nil, nil, errors.New("cannot resolve native capture hostname")
	}
	hostname := output.Hostname(actualHostname)
	// Core checks still consult the global infrastructure-mode setting when
	// constructing their metric tags. Delivery uses only the isolated config.
	setup.Datadog().Set("infrastructure_mode", "end_user_device", configmodel.SourceAgentRuntime)
	setup.Datadog().Set("telemetry.enabled", false, configmodel.SourceAgentRuntime)
	env.DetectFeatures(p.Config)
	tagger := taggerimpl.NewComponent()
	ha, err := haagent.NewComponent(haagent.Requires{Logger: logger, AgentConfig: p.Config, Hostname: hostname})
	if err != nil {
		return nil, nil, err
	}
	opts := captureDemultiplexerOptions()
	opts.CaptureTransformer = s
	demux := aggregator.InitAndStartAgentDemultiplexer(logger, p.Serializer.Forwarder, nil, opts, noEvents{}, ha.Comp, selector.FromConfig(p.Config), tagger, filterlist.NewNoopFilterList(), "capture-host")
	var nativeChecks []check.Check
	lc := &lifecycle{}
	cleanup := func() {
		for _, c := range nativeChecks {
			c.Cancel()
		}
		demux.Stop()
		lc.stop()
	}
	failed := true
	defer func() {
		if failed {
			cleanup()
		}
	}()
	for _, factory := range []option.Option[func() check.Check]{cpu.Factory(), memory.Factory(), wlan.Factory()} {
		newCheck, ok := factory.Get()
		if !ok {
			continue
		}
		c := newCheck()
		if err := c.Configure(demux, 0, nil, nil, "eudm-capture", "file"); err != nil {
			return nil, nil, fmt.Errorf("cannot configure native %s check", c.String())
		}
		nativeChecks = append(nativeChecks, c)
	}
	metricInterval := aggregator.DefaultFlushInterval
	collectors := []scheduledCollector{{name: "metrics", interval: metricInterval, run: func(context.Context) (time.Duration, error) {
		for _, c := range nativeChecks {
			_ = c.Run()
		}
		demux.ForceFlushToSerializer(time.Now(), true, true)
		return metricInterval, nil
	}}}
	metadata := hostimpl.NewComponent(hostimpl.Requires{Log: logger, Config: p.Config, Resources: noResources{}, Serializer: p.Serializer, Hostname: hostname})
	collectors = append(collectors, scheduledCollector{name: "host_metadata", interval: 30 * time.Minute, run: func(ctx context.Context) (time.Duration, error) { return metadata.MetadataProvider.Callback(ctx), nil }})
	// Use the native Agent collectors, including complete software snapshots.
	// The cadence follows softwareinventory's configured minimum of ten minutes.
	softwareInterval := time.Duration(max(p.Config.GetInt("software_inventory.interval"), 10)) * time.Minute
	collectors = append(collectors, scheduledCollector{name: "software", interval: softwareInterval, run: func(ctx context.Context) (time.Duration, error) {
		entries, _, err := software.GetSoftwareInventory()
		if err != nil || len(entries) == 0 {
			return softwareInterval, errNativeCollection
		}
		values := make([]software.Entry, 0, len(entries))
		for _, entry := range entries {
			if entry != nil {
				values = append(values, *entry)
			}
		}
		clean := s.sanitizer.Software(values)
		for _, item := range clean {
			if !slices.Contains(s.profile.SoftwareNames, item.DisplayName) {
				s.profile.SoftwareNames = append(s.profile.SoftwareNames, item.DisplayName)
			}
		}
		payload := &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: clean}}
		if err := s.add(schema.Software, payload); err != nil {
			return softwareInterval, err
		}
		data, err := payload.MarshalJSON()
		if err != nil {
			return softwareInterval, errors.New("cannot encode sanitized software snapshot")
		}
		return softwareInterval, p.Event(ctx, eventplatform.EventTypeSoftwareInventory, data, time.Now())
	}})
	wmeta := workloadmetaimpl.NewComponent(workloadmetaimpl.Dependencies{Lc: lc, Log: logger, Config: p.Config, Params: workloadmeta.Params{}}).Comp
	filters, err := filterimpl.NewComponent(filterimpl.Requires{Config: p.Config, Log: logger, Telemetry: telemetryimpl.GetCompatComponent()})
	if err != nil {
		return nil, nil, err
	}
	containers.InitSharedContainerProvider(wmeta, tagger, filters.Comp)
	info, err := checks.CollectSystemInfo()
	if err != nil {
		return nil, nil, errors.New("cannot collect native process system information")
	}
	hostInfo := &checks.HostInfo{HostName: actualHostname, SystemInfo: info}
	syscfg := &checks.SysProbeConfig{SystemProbeAddress: p.Config.GetString("system_probe_config.sysprobe_socket"), MaxConnsPerMessage: 600}
	process := checks.NewProcessCheck(p.Config, p.Config, wmeta, gpusubscriberimpl.NoopSubscriber{}, &statsd.NoOpClient{}, nil, tagger)
	if err := process.Init(syscfg, hostInfo, true); err != nil {
		return nil, nil, errors.New("cannot initialize native process check")
	}
	lc.Append(compdef.Hook{OnStop: func(context.Context) error { process.Cleanup(); return nil }})
	var groupID int32
	addProcessCheck := func(c checks.Check) {
		interval := checks.GetInterval(p.Config, c.Name())
		collectors = append(collectors, scheduledCollector{name: c.Name(), interval: interval, run: func(ctx context.Context) (time.Duration, error) {
			result, err := c.Run(func() int32 { groupID++; return groupID }, nil)
			if err != nil {
				return interval, errNativeCollection
			}
			if result == nil || len(result.Payloads()) == 0 {
				return interval, nil
			}
			// Empty cycles are legitimate on a quiet device, but are not evidence
			// of complete coverage. Wait for actual records on later native cycles.
			var payloads []model.MessageBody
			for _, message := range result.Payloads() {
				switch message := message.(type) {
				case *model.CollectorProc:
					if message != nil && len(message.Processes) > 0 {
						payloads = append(payloads, message)
					}
				case *model.CollectorConnections:
					if message != nil && len(message.Connections) > 0 {
						payloads = append(payloads, message)
					}
				}
			}
			if len(payloads) == 0 {
				return interval, nil
			}
			return interval, p.Submitter.SubmitForHost(ctx, time.Now(), c.Name(), "capture-host", &types.Payload{Message: payloads})
		}})
	}
	addProcessCheck(process)
	connection, err := connectionCollector(p.Config, syscfg, hostInfo, wmeta, tagger)
	if err != nil {
		return nil, nil, err
	}
	if connection != nil {
		addProcessCheck(connection)
		lc.Append(compdef.Hook{OnStop: func(context.Context) error { connection.Cleanup(); return nil }})
	}
	for _, hook := range lc.hooks {
		if hook.OnStart != nil {
			if err := hook.OnStart(ctx); err != nil {
				return nil, nil, err
			}
		}
	}
	failed = false
	return collectors, cleanup, nil
}

// The demultiplexer emits its own service checks and diagnostic series. Only
// the explicitly transformed series may leave it during capture. Set these
// before construction because the serializers snapshot their configuration.
func captureDemultiplexerOptions() aggregator.AgentDemultiplexerOptions {
	for _, payload := range []string{"events", "service_checks", "sketches"} {
		setup.Datadog().Set("enable_payloads."+payload, false, configmodel.SourceAgentRuntime)
	}
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 0
	opts.DontStartForwarders = true
	return opts
}
