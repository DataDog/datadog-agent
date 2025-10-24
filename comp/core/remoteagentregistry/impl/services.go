// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"google.golang.org/grpc"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	registryutil "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/util"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func (ra *remoteAgentRegistry) fillFlare(builder flarebuilder.FlareBuilder) error {
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
			return struct{}{}
		}
		if promText, ok := resp.Payload.(*pb.GetTelemetryResponse_PromText); ok {
			collectFromPromText(ch, promText.PromText, details.SanitizedDisplayName)
		}
		return struct{}{}
	}

	// We don't need to collect any value since everything is sent through the provided channel
	_ = callAgentsForService(c.registry, TelemetryServiceName, client, processor)
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

		// Prefix the name with the registry name
		// name := proto.String(fmt.Sprintf("%s__%s", remoteAgentName, *mf.Name))

		var errs []error

		for _, metric := range mf.Metric {
			if metric == nil {
				continue
			}

			labelNames := make([]string, 0, len(metric.Label)+1)
			labelValues := make([]string, 0, len(metric.Label)+1)
			labelNames = append(labelNames, "remote_agent")
			labelValues = append(labelValues, remoteAgentName)
			for _, label := range metric.Label {
				labelNames = append(labelNames, *label.Name)
				labelValues = append(labelValues, *label.Value)
			}
			switch *mf.Type {
			case dto.MetricType_COUNTER:
				metric, err := prometheus.NewConstMetric(
					prometheus.NewDesc(*mf.Name, help, labelNames, nil),
					prometheus.CounterValue,
					*metric.Counter.Value,
					labelValues...,
				)
				if err != nil {
					errs = append(errs, err)
				}
				ch <- metric
			case dto.MetricType_GAUGE:
				metric, err := prometheus.NewConstMetric(
					prometheus.NewDesc(*mf.Name, help, labelNames, nil),
					prometheus.GaugeValue,
					*metric.Gauge.Value,
					labelValues...,
				)
				if err != nil {
					errs = append(errs, err)
				}
				ch <- metric

			case dto.MetricType_SUMMARY:
				quantiles := make(map[float64]float64)
				for _, quantile := range metric.Summary.Quantile {
					quantiles[*quantile.Quantile] = *quantile.Value
				}
				metric, err := prometheus.NewConstSummary(
					prometheus.NewDesc(*mf.Name, help, labelNames, nil),
					*metric.Summary.SampleCount,
					*metric.Summary.SampleSum,
					quantiles,
					labelValues...,
				)
				if err != nil {
					errs = append(errs, err)
				}
				ch <- metric

			case dto.MetricType_HISTOGRAM:
				buckets := make(map[float64]uint64)
				for _, quantile := range metric.Histogram.Bucket {
					buckets[*quantile.UpperBound] = *quantile.CumulativeCount
				}
				metric, err := prometheus.NewConstHistogram(
					prometheus.NewDesc(*mf.Name, help, labelNames, nil),
					*metric.Histogram.SampleCount,
					*metric.Histogram.SampleSum,
					buckets,
					labelValues...,
				)
				if err != nil {
					errs = append(errs, err)
				}
				ch <- metric

			default:
				log.Warnf("Unknown metric type: %s", *mf.Type)
			}
		}
		if len(errs) > 0 {
			log.Warnf("Failed to collect telemetry metrics: %v", errs)
		}
	}
}
