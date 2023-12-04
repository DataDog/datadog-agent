// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
	"github.com/tinylib/msgp/msgp"
)

var _ otlpmetrics.Consumer = (*serializerConsumer)(nil)

type serializerConsumer struct {
	cardinality collectors.TagCardinality
	extraTags   []string
	series      metrics.Series
	sketches    metrics.SketchSeriesList
	apmstats    []io.Reader
}

// enrichedTags of a given dimension.
// In the OTLP pipeline, 'contexts' are kept within the translator and function differently than DogStatsD/check metrics.
func (c *serializerConsumer) enrichedTags(dimensions *otlpmetrics.Dimensions) []string {
	enrichedTags := make([]string, 0, len(c.extraTags)+len(dimensions.Tags()))
	enrichedTags = append(enrichedTags, c.extraTags...)
	enrichedTags = append(enrichedTags, dimensions.Tags()...)

	entityTags, err := tagger.Tag(dimensions.OriginID(), c.cardinality)
	if err != nil {
		log.Tracef("Cannot get tags for entity %s: %s", dimensions.OriginID(), err)
	} else {
		enrichedTags = append(enrichedTags, entityTags...)
	}

	globalTags, err := tagger.GlobalTags(c.cardinality)
	if err != nil {
		log.Trace(err.Error())
	} else {
		enrichedTags = append(enrichedTags, globalTags...)
	}

	return enrichedTags
}

func (c *serializerConsumer) ConsumeAPMStats(ss *pb.ClientStatsPayload) {
	log.Tracef("Serializing %d client stats buckets.", len(ss.Stats))
	ss.Tags = append(ss.Tags, c.extraTags...)
	body := new(bytes.Buffer)
	if err := msgp.Encode(body, ss); err != nil {
		log.Errorf("Error encoding ClientStatsPayload: %v", err)
		return
	}
	c.apmstats = append(c.apmstats, body)
}

func originServiceToMetricsSource(service otlpmetrics.OriginService) metrics.MetricSource {
	switch service {
	case otlpmetrics.OriginServiceUnknown:
		return metrics.MetricSourceOTLP
	case otlpmetrics.OriginServiceActiveDirectoryDSReceiver:
		return metrics.MetricSourceOTelActiveDirectoryDSReceiver
	case otlpmetrics.OriginServiceAerospikeReceiver:
		return metrics.MetricSourceOTelAerospikeReceiver
	case otlpmetrics.OriginServiceApacheReceiver:
		return metrics.MetricSourceOTelApacheReceiver
	case otlpmetrics.OriginServiceApacheSparkReceiver:
		return metrics.MetricSourceOTelApacheSparkReceiver
	case otlpmetrics.OriginServiceAzureMonitorReceiver:
		return metrics.MetricSourceOTelAzureMonitorReceiver
	case otlpmetrics.OriginServiceBigIPReceiver:
		return metrics.MetricSourceOTelBigIPReceiver
	case otlpmetrics.OriginServiceChronyReceiver:
		return metrics.MetricSourceOTelChronyReceiver
	case otlpmetrics.OriginServiceCouchDBReceiver:
		return metrics.MetricSourceOTelCouchDBReceiver
	case otlpmetrics.OriginServiceDockerStatsReceiver:
		return metrics.MetricSourceOTelDockerStatsReceiver
	case otlpmetrics.OriginServiceElasticsearchReceiver:
		return metrics.MetricSourceOTelElasticsearchReceiver
	case otlpmetrics.OriginServiceExpVarReceiver:
		return metrics.MetricSourceOTelExpVarReceiver
	case otlpmetrics.OriginServiceFileStatsReceiver:
		return metrics.MetricSourceOTelFileStatsReceiver
	case otlpmetrics.OriginServiceFlinkMetricsReceiver:
		return metrics.MetricSourceOTelFlinkMetricsReceiver
	case otlpmetrics.OriginServiceGitProviderReceiver:
		return metrics.MetricSourceOTelGitProviderReceiver
	case otlpmetrics.OriginServiceHAProxyReceiver:
		return metrics.MetricSourceOTelHAProxyReceiver
	case otlpmetrics.OriginServiceHostMetricsReceiver:
		return metrics.MetricSourceOTelHostMetricsReceiver
	case otlpmetrics.OriginServiceHTTPCheckReceiver:
		return metrics.MetricSourceOTelHTTPCheckReceiver
	case otlpmetrics.OriginServiceIISReceiver:
		return metrics.MetricSourceOTelIISReceiver
	case otlpmetrics.OriginServiceK8SClusterReceiver:
		return metrics.MetricSourceOTelK8SClusterReceiver
	case otlpmetrics.OriginServiceKafkaMetricsReceiver:
		return metrics.MetricSourceOTelKafkaMetricsReceiver
	case otlpmetrics.OriginServiceKubeletStatsReceiver:
		return metrics.MetricSourceOTelKubeletStatsReceiver
	case otlpmetrics.OriginServiceMemcachedReceiver:
		return metrics.MetricSourceOTelMemcachedReceiver
	case otlpmetrics.OriginServiceMongoDBAtlasReceiver:
		return metrics.MetricSourceOTelMongoDBAtlasReceiver
	case otlpmetrics.OriginServiceMongoDBReceiver:
		return metrics.MetricSourceOTelMongoDBReceiver
	case otlpmetrics.OriginServiceMySQLReceiver:
		return metrics.MetricSourceOTelMySQLReceiver
	case otlpmetrics.OriginServiceNginxReceiver:
		return metrics.MetricSourceOTelNginxReceiver
	case otlpmetrics.OriginServiceNSXTReceiver:
		return metrics.MetricSourceOTelNSXTReceiver
	case otlpmetrics.OriginServiceOracleDBReceiver:
		return metrics.MetricSourceOTelOracleDBReceiver
	case otlpmetrics.OriginServicePostgreSQLReceiver:
		return metrics.MetricSourceOTelPostgreSQLReceiver
	case otlpmetrics.OriginServicePrometheusReceiver:
		return metrics.MetricSourceOTelPrometheusReceiver
	case otlpmetrics.OriginServiceRabbitMQReceiver:
		return metrics.MetricSourceOTelRabbitMQReceiver
	case otlpmetrics.OriginServiceRedisReceiver:
		return metrics.MetricSourceOTelRedisReceiver
	case otlpmetrics.OriginServiceRiakReceiver:
		return metrics.MetricSourceOTelRiakReceiver
	case otlpmetrics.OriginServiceSAPHANAReceiver:
		return metrics.MetricSourceOTelSAPHANAReceiver
	case otlpmetrics.OriginServiceSNMPReceiver:
		return metrics.MetricSourceOTelSNMPReceiver
	case otlpmetrics.OriginServiceSnowflakeReceiver:
		return metrics.MetricSourceOTelSnowflakeReceiver
	case otlpmetrics.OriginServiceSplunkEnterpriseReceiver:
		return metrics.MetricSourceOTelSplunkEnterpriseReceiver
	case otlpmetrics.OriginServiceSQLServerReceiver:
		return metrics.MetricSourceOTelSQLServerReceiver
	case otlpmetrics.OriginServiceSSHCheckReceiver:
		return metrics.MetricSourceOTelSSHCheckReceiver
	case otlpmetrics.OriginServiceStatsDReceiver:
		return metrics.MetricSourceOTelStatsDReceiver
	case otlpmetrics.OriginServiceVCenterReceiver:
		return metrics.MetricSourceOTelVCenterReceiver
	case otlpmetrics.OriginServiceZookeeperReceiver:
		return metrics.MetricSourceOTelZookeeperReceiver
	}

	return metrics.MetricSourceOTLP
}

func (c *serializerConsumer) ConsumeSketch(_ context.Context, dimensions *otlpmetrics.Dimensions, ts uint64, qsketch *quantile.Sketch) {
	c.sketches = append(c.sketches, &metrics.SketchSeries{
		Name:     dimensions.Name(),
		Tags:     tagset.CompositeTagsFromSlice(c.enrichedTags(dimensions)),
		Host:     dimensions.Host(),
		Source:   originServiceToMetricsSource(dimensions.OriginService()),
		Interval: 0, // OTLP metrics do not have an interval.
		Points: []metrics.SketchPoint{{
			Ts:     int64(ts / 1e9),
			Sketch: qsketch,
		}},
	})
}

func apiTypeFromTranslatorType(typ otlpmetrics.DataType) metrics.APIMetricType {
	switch typ {
	case otlpmetrics.Count:
		return metrics.APICountType
	case otlpmetrics.Gauge:
		return metrics.APIGaugeType
	}
	panic(fmt.Sprintf("unreachable: received non-count non-gauge type: %d", typ))
}

func (c *serializerConsumer) ConsumeTimeSeries(ctx context.Context, dimensions *otlpmetrics.Dimensions, typ otlpmetrics.DataType, ts uint64, value float64) { //nolint:revive // TODO fix revive unused-parameter
	c.series = append(c.series,
		&metrics.Serie{
			Name:     dimensions.Name(),
			Points:   []metrics.Point{{Ts: float64(ts / 1e9), Value: value}},
			Tags:     tagset.CompositeTagsFromSlice(c.enrichedTags(dimensions)),
			Host:     dimensions.Host(),
			MType:    apiTypeFromTranslatorType(typ),
			Source:   originServiceToMetricsSource(dimensions.OriginService()),
			Interval: 0, // OTLP metrics do not have an interval.
		},
	)
}

// addTelemetryMetric to know if an Agent is using OTLP metrics.
func (c *serializerConsumer) addTelemetryMetric(hostname string) {
	c.series = append(c.series, &metrics.Serie{
		Name:           "datadog.agent.otlp.metrics",
		Points:         []metrics.Point{{Value: 1, Ts: float64(time.Now().Unix())}},
		Tags:           tagset.CompositeTagsFromSlice([]string{}),
		Host:           hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	})
}

// addRuntimeTelemetryMetric to know if an Agent is using OTLP runtime metrics.
func (c *serializerConsumer) addRuntimeTelemetryMetric(hostname string, languageTags []string) {
	for _, lang := range languageTags {
		c.series = append(c.series, &metrics.Serie{
			Name:           "datadog.agent.otlp.runtime_metrics",
			Points:         []metrics.Point{{Value: 1, Ts: float64(time.Now().Unix())}},
			Tags:           tagset.CompositeTagsFromSlice([]string{fmt.Sprintf("language:%v", lang)}),
			Host:           hostname,
			MType:          metrics.APIGaugeType,
			SourceTypeName: "System",
		})
	}
}

// Send exports all data recorded by the consumer. It does not reset the consumer.
func (c *serializerConsumer) Send(s serializer.MetricSerializer) error {
	var serieErr, sketchesErr error
	metrics.Serialize(
		metrics.NewIterableSeries(func(se *metrics.Serie) {}, 200, 4000),
		metrics.NewIterableSketches(func(se *metrics.SketchSeries) {}, 200, 4000),
		func(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
			for _, serie := range c.series {
				seriesSink.Append(serie)
			}
			for _, sketch := range c.sketches {
				sketchesSink.Append(sketch)
			}
		}, func(serieSource metrics.SerieSource) {
			serieErr = s.SendIterableSeries(serieSource)
		}, func(sketchesSource metrics.SketchesSource) {
			sketchesErr = s.SendSketch(sketchesSource)
		},
	)

	apmErr := c.sendAPMStats()
	return multierr.Combine(serieErr, sketchesErr, apmErr)
}

func (c *serializerConsumer) sendAPMStats() error {
	addr := fmt.Sprintf("http://localhost:%s/v0.6/stats", config.Datadog.GetString("apm_config.receiver_port"))
	log.Debugf("Exporting %d APM stats payloads", len(c.apmstats))
	for _, body := range c.apmstats {
		resp, err := http.Post(addr, "application/msgpack", body)
		if err != nil {
			return fmt.Errorf("could not flush StatsPayload: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			peek := make([]byte, 1024)
			n, _ := resp.Body.Read(peek)
			return fmt.Errorf("could not flush StatsPayload: HTTP Status code == %s %s", resp.Status, string(peek[:n]))
		}
	}
	return nil
}
