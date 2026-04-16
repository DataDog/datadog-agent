// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	registryutil "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/util"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// remoteAgentMetricTagName is the name of the label that will be added to all metrics coming from the remote agent
	remoteAgentMetricTagName = "remote_agent"
)

func (ra *remoteAgentRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetStatusDetailsResponse, error) {
		return remoteAgent.GetStatusDetails(ctx, &pb.GetStatusDetailsRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, in *pb.GetStatusDetailsResponse, err error) remoteagentregistry.StatusData {
		out := remoteagentregistry.StatusData{
			RegisteredAgent: details,
		}

		if err != nil {
			out.FailureReason = fmt.Sprintf("Failed to query for status: %v", err)
			return out
		}

		// converting main section
		if in.MainSection != nil {
			out.MainSection = in.MainSection.Fields
		}

		// converting named sections
		sections := make(map[string]remoteagentregistry.StatusSection, len(in.NamedSections))
		for name, section := range in.NamedSections {
			if section != nil {
				sections[name] = section.Fields
			}
		}
		out.NamedSections = sections

		return out
	}

	return callAgentsForService(ra, StatusServiceName, client, processor)
}

func (ra *remoteAgentRegistry) fillFlare(_ context.Context, builder flarebuilder.FlareBuilder) error {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetFlareFilesResponse, error) {
		return remoteAgent.GetFlareFiles(ctx, &pb.GetFlareFilesRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, resp *pb.GetFlareFilesResponse, err error) *remoteagentregistry.FlareData {
		if err != nil {
			return nil
		}
		return &remoteagentregistry.FlareData{
			RegisteredAgent: details,
			Files:           resp.Files,
		}
	}

	// We've collected all the flare data we can, so now we add it to the flare builder.
	for _, flareData := range callAgentsForService(ra, FlareServiceName, client, processor) {
		if flareData == nil {
			continue
		}

		for fileName, fileData := range flareData.Files {
			// The flare builder already logs errors, so we can ignore them here.
			// an error here should not prevent the flare from being created.
			//nolint:errcheck
			builder.AddFile(fmt.Sprintf("%s/%s", flareData.RegisteredAgent.String(), registryutil.SanitizeFileName(fileName)), fileData)
		}
	}

	return nil
}

func (ra *remoteAgentRegistry) GetAllRemoteCommands() []remoteagentregistry.CommandData {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	var commands []remoteagentregistry.CommandData
	for _, client := range ra.agentMap {
		if !slices.Contains(client.services, CommandServiceName) {
			continue
		}
		if len(client.cachedCommands) == 0 {
			continue
		}
		commands = append(commands, remoteagentregistry.CommandData{
			RegisteredAgent: client.RegisteredAgent,
			Commands:        client.cachedCommands,
		})
	}
	return commands
}

func (ra *remoteAgentRegistry) ExecuteRemoteCommand(commandPath string, request *pb.ExecuteCommandRequest) (*remoteagentregistry.CommandResult, error) {
	ra.agentMapMu.Lock()

	var targetClient *remoteAgentClient
	for _, client := range ra.agentMap {
		if !slices.Contains(client.services, CommandServiceName) {
			continue
		}
		if findCommand(client.cachedCommands, commandPath) != nil {
			targetClient = client
			break
		}
	}

	ra.agentMapMu.Unlock()

	if targetClient == nil {
		return nil, fmt.Errorf("no remote agent provides command %q", commandPath)
	}

	queryTimeout := ra.conf.GetDuration("remote_agent.registry.query_timeout")
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	start := time.Now()
	var responseHeader metadata.MD
	resp, err := targetClient.ExecuteCommand(ctx, request, grpc.WaitForReady(true), grpc.Header(&responseHeader))

	ra.telemetryStore.remoteAgentActionDuration.Observe(
		time.Since(start).Seconds(),
		targetClient.RegisteredAgent.SanitizedDisplayName,
		CommandServiceName,
	)

	if err != nil {
		ra.telemetryStore.remoteAgentActionError.Inc(targetClient.RegisteredAgent.SanitizedDisplayName, CommandServiceName, grpcErrorMessage(err))
		return nil, fmt.Errorf("failed to execute command %q on remote agent '%s': %w", commandPath, targetClient.RegisteredAgent.DisplayName, err)
	}

	// Validate session ID
	if validationErr := targetClient.validateSessionID(responseHeader); validationErr != nil {
		ra.telemetryStore.remoteAgentActionError.Inc(targetClient.RegisteredAgent.SanitizedDisplayName, CommandServiceName, sessionIDMismatch)
		targetClient.unhealthy = true
		targetClient.unhealthyReason = validationErr
		return nil, validationErr
	}

	return &remoteagentregistry.CommandResult{
		ExitCode:     resp.ExitCode,
		Stdout:       resp.Stdout,
		Stderr:       resp.Stderr,
		BinaryOutput: resp.BinaryOutput,
	}, nil
}

// findCommand searches through a list of commands and their children for a command matching the given path.
// Matches on both the command name and its alias.
func findCommand(commands []*pb.Command, path string) *pb.Command {
	for _, cmd := range commands {
		if cmd.Name == path || (cmd.Alias != "" && cmd.Alias == path) {
			return cmd
		}
		// Search children recursively
		if found := findCommand(cmd.Children, path); found != nil {
			return found
		}
	}
	return nil
}

type registryCollector struct {
	registry       *remoteAgentRegistry
	telemetryStore *telemetryStore
}

func newRegistryCollector(registry *remoteAgentRegistry, telemetryStore *telemetryStore) prometheus.Collector {
	return &registryCollector{
		registry:       registry,
		telemetryStore: telemetryStore,
	}
}

func (ra *remoteAgentRegistry) registerCollector() {
	ra.telemetry.RegisterCollector(newRegistryCollector(ra, ra.telemetryStore))
}

func (c *registryCollector) Describe(_ chan<- *prometheus.Desc) {
}

func (c *registryCollector) Collect(ch chan<- prometheus.Metric) {
	c.GetRegisteredAgentsTelemetry(ch)
}

func (c *registryCollector) GetRegisteredAgentsTelemetry(ch chan<- prometheus.Metric) {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetTelemetryResponse, error) {
		return remoteAgent.GetTelemetry(ctx, &pb.GetTelemetryRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, resp *pb.GetTelemetryResponse, err error) struct{} {
		if err != nil {
			log.Warnf("Failed to collect telemetry metrics from remoteAgent %v: %v", details.SanitizedDisplayName, err)
			return struct{}{}
		}
		if promText, ok := resp.Payload.(*pb.GetTelemetryResponse_PromText); ok {
			collectFromPromText(ch, promText.PromText, details.SanitizedDisplayName)
		}
		return struct{}{}
	}

	// We don't need to collect any value since everything is sent through the provided channel
	callAgentsForService(c.registry, TelemetryServiceName, client, processor)
}

// Retrieve the telemetry data in exposition format from the remote agent
func collectFromPromText(ch chan<- prometheus.Metric, promText string, remoteAgentName string) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(promText))
	if err != nil {
		log.Warnf("Failed to parse prometheus text: %v", err)
		return
	}

	for _, mf := range metricFamilies {
		help := ""
		if mf.Help != nil {
			help = *mf.Help
		}

		for _, metric := range mf.Metric {
			if metric == nil {
				continue
			}

			// Check if the metric already has a remote_agent label.
			// With explicit agent identity, metrics should already have the correct value.
			// We only add the label if it's missing (for backward compatibility).
			hasRemoteAgentLabel := slices.ContainsFunc(metric.Label, func(label *dto.LabelPair) bool {
				return *label.Name == remoteAgentMetricTagName
			})

			labelNames := make([]string, 0, len(metric.Label)+1)
			labelValues := make([]string, 0, len(metric.Label)+1)
			// Only add remote_agent label if the metric doesn't already have one
			if !hasRemoteAgentLabel {
				labelNames = append(labelNames, remoteAgentMetricTagName)
				labelValues = append(labelValues, remoteAgentName)
			}
			for _, label := range metric.Label {
				labelNames = append(labelNames, *label.Name)
				labelValues = append(labelValues, *label.Value)
			}

			desc := prometheus.NewDesc(*mf.Name, help, labelNames, nil)

			switch *mf.Type {
			case dto.MetricType_COUNTER:
				value := *metric.Counter.Value

				metric, err := prometheus.NewConstMetric(desc, prometheus.CounterValue, value, labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry counter metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric
			case dto.MetricType_GAUGE:
				value := *metric.Gauge.Value

				metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value, labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry gauge metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric

			case dto.MetricType_SUMMARY:
				log.Warnf("Dropping metrics %v from remoteAgent %v: unimplemented summary aggregation logic", mf.GetName(), remoteAgentName)
				continue

			case dto.MetricType_HISTOGRAM:
				count := metric.Histogram.GetSampleCount()
				sum := metric.Histogram.GetSampleSum()
				buckets := make(map[float64]uint64)
				for _, bucket := range metric.Histogram.GetBucket() {
					buckets[bucket.GetUpperBound()] = bucket.GetCumulativeCount()
				}

				metric, err := prometheus.NewConstHistogram(desc, count, sum, buckets, labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry histogram metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric

			default:
				log.Warnf("Dropping metrics %v from remoteAgent %v: unknown metric type %s", mf.GetName(), remoteAgentName, mf.GetType())
			}
		}
	}
}
