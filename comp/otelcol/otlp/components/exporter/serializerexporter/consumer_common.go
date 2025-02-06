// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
)

var metricOriginsMappings = map[otlpmetrics.OriginProductDetail]metrics.MetricSource{
	otlpmetrics.OriginProductDetailUnknown:                   metrics.MetricSourceOpenTelemetryCollectorUnknown,
	otlpmetrics.OriginProductDetailDockerStatsReceiver:       metrics.MetricSourceOpenTelemetryCollectorDockerstatsReceiver,
	otlpmetrics.OriginProductDetailElasticsearchReceiver:     metrics.MetricSourceOpenTelemetryCollectorElasticsearchReceiver,
	otlpmetrics.OriginProductDetailExpVarReceiver:            metrics.MetricSourceOpenTelemetryCollectorExpvarReceiver,
	otlpmetrics.OriginProductDetailFileStatsReceiver:         metrics.MetricSourceOpenTelemetryCollectorFilestatsReceiver,
	otlpmetrics.OriginProductDetailFlinkMetricsReceiver:      metrics.MetricSourceOpenTelemetryCollectorFlinkmetricsReceiver,
	otlpmetrics.OriginProductDetailGitProviderReceiver:       metrics.MetricSourceOpenTelemetryCollectorGitproviderReceiver,
	otlpmetrics.OriginProductDetailHAProxyReceiver:           metrics.MetricSourceOpenTelemetryCollectorHaproxyReceiver,
	otlpmetrics.OriginProductDetailHostMetricsReceiver:       metrics.MetricSourceOpenTelemetryCollectorHostmetricsReceiver,
	otlpmetrics.OriginProductDetailHTTPCheckReceiver:         metrics.MetricSourceOpenTelemetryCollectorHttpcheckReceiver,
	otlpmetrics.OriginProductDetailIISReceiver:               metrics.MetricSourceOpenTelemetryCollectorIisReceiver,
	otlpmetrics.OriginProductDetailK8SClusterReceiver:        metrics.MetricSourceOpenTelemetryCollectorK8sclusterReceiver,
	otlpmetrics.OriginProductDetailKafkaMetricsReceiver:      metrics.MetricSourceOpenTelemetryCollectorKafkametricsReceiver,
	otlpmetrics.OriginProductDetailKubeletStatsReceiver:      metrics.MetricSourceOpenTelemetryCollectorKubeletstatsReceiver,
	otlpmetrics.OriginProductDetailMemcachedReceiver:         metrics.MetricSourceOpenTelemetryCollectorMemcachedReceiver,
	otlpmetrics.OriginProductDetailMongoDBAtlasReceiver:      metrics.MetricSourceOpenTelemetryCollectorMongodbatlasReceiver,
	otlpmetrics.OriginProductDetailMongoDBReceiver:           metrics.MetricSourceOpenTelemetryCollectorMongodbReceiver,
	otlpmetrics.OriginProductDetailMySQLReceiver:             metrics.MetricSourceOpenTelemetryCollectorMysqlReceiver,
	otlpmetrics.OriginProductDetailNginxReceiver:             metrics.MetricSourceOpenTelemetryCollectorNginxReceiver,
	otlpmetrics.OriginProductDetailNSXTReceiver:              metrics.MetricSourceOpenTelemetryCollectorNsxtReceiver,
	otlpmetrics.OriginProductDetailOracleDBReceiver:          metrics.MetricSourceOpenTelemetryCollectorOracledbReceiver,
	otlpmetrics.OriginProductDetailPostgreSQLReceiver:        metrics.MetricSourceOpenTelemetryCollectorPostgresqlReceiver,
	otlpmetrics.OriginProductDetailPrometheusReceiver:        metrics.MetricSourceOpenTelemetryCollectorPrometheusReceiver,
	otlpmetrics.OriginProductDetailRabbitMQReceiver:          metrics.MetricSourceOpenTelemetryCollectorRabbitmqReceiver,
	otlpmetrics.OriginProductDetailRedisReceiver:             metrics.MetricSourceOpenTelemetryCollectorRedisReceiver,
	otlpmetrics.OriginProductDetailRiakReceiver:              metrics.MetricSourceOpenTelemetryCollectorRiakReceiver,
	otlpmetrics.OriginProductDetailSAPHANAReceiver:           metrics.MetricSourceOpenTelemetryCollectorSaphanaReceiver,
	otlpmetrics.OriginProductDetailSNMPReceiver:              metrics.MetricSourceOpenTelemetryCollectorSnmpReceiver,
	otlpmetrics.OriginProductDetailSnowflakeReceiver:         metrics.MetricSourceOpenTelemetryCollectorSnowflakeReceiver,
	otlpmetrics.OriginProductDetailSplunkEnterpriseReceiver:  metrics.MetricSourceOpenTelemetryCollectorSplunkenterpriseReceiver,
	otlpmetrics.OriginProductDetailSQLServerReceiver:         metrics.MetricSourceOpenTelemetryCollectorSqlserverReceiver,
	otlpmetrics.OriginProductDetailSSHCheckReceiver:          metrics.MetricSourceOpenTelemetryCollectorSshcheckReceiver,
	otlpmetrics.OriginProductDetailStatsDReceiver:            metrics.MetricSourceOpenTelemetryCollectorStatsdReceiver,
	otlpmetrics.OriginProductDetailVCenterReceiver:           metrics.MetricSourceOpenTelemetryCollectorVcenterReceiver,
	otlpmetrics.OriginProductDetailZookeeperReceiver:         metrics.MetricSourceOpenTelemetryCollectorZookeeperReceiver,
	otlpmetrics.OriginProductDetailActiveDirectoryDSReceiver: metrics.MetricSourceOpenTelemetryCollectorActiveDirectorydsReceiver,
	otlpmetrics.OriginProductDetailAerospikeReceiver:         metrics.MetricSourceOpenTelemetryCollectorAerospikeReceiver,
	otlpmetrics.OriginProductDetailApacheReceiver:            metrics.MetricSourceOpenTelemetryCollectorApacheReceiver,
	otlpmetrics.OriginProductDetailApacheSparkReceiver:       metrics.MetricSourceOpenTelemetryCollectorApachesparkReceiver,
	otlpmetrics.OriginProductDetailAzureMonitorReceiver:      metrics.MetricSourceOpenTelemetryCollectorAzuremonitorReceiver,
	otlpmetrics.OriginProductDetailBigIPReceiver:             metrics.MetricSourceOpenTelemetryCollectorBigipReceiver,
	otlpmetrics.OriginProductDetailChronyReceiver:            metrics.MetricSourceOpenTelemetryCollectorChronyReceiver,
	otlpmetrics.OriginProductDetailCouchDBReceiver:           metrics.MetricSourceOpenTelemetryCollectorCouchdbReceiver,
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
