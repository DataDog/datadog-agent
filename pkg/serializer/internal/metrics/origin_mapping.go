// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func MetricSourceToOriginCategory(ms metrics.MetricSource) int32 {
	// These constants map to specific fields in the 'OriginCategory' enum in origin.proto
	switch ms {
	case metrics.MetricSourceUnknown:
		return 0
	case metrics.MetricSourceDogstatsd:
		return 10
	case metrics.MetricSourceJmxCustom,
		metrics.MetricSourceActivemq,
		metrics.MetricSourceCassandra,
		metrics.MetricSourceConfluentPlatform,
		metrics.MetricSourceHazelcast,
		metrics.MetricSourceHive,
		metrics.MetricSourceHivemq,
		metrics.MetricSourceHudi,
		metrics.MetricSourceIgnite,
		metrics.MetricSourceJbossWildfly,
		metrics.MetricSourceKafka,
		metrics.MetricSourcePresto,
		metrics.MetricSourceSolr,
		metrics.MetricSourceSonarqube,
		metrics.MetricSourceTomcat,
		metrics.MetricSourceWeblogic,
		metrics.MetricSourceContainer,
		metrics.MetricSourceContainerd,
		metrics.MetricSourceCri,
		metrics.MetricSourceDocker,
		metrics.MetricSourceNtp,
		metrics.MetricSourceSystemd,
		metrics.MetricSourceHelm,
		metrics.MetricSourceKubernetesAPIServer,
		metrics.MetricSourceKubernetesStateCore,
		metrics.MetricSourceOrchestrator,
		metrics.MetricSourceWinproc,
		metrics.MetricSourceFileHandle,
		metrics.MetricSourceWinkmem,
		metrics.MetricSourceIo,
		metrics.MetricSourceUptime,
		metrics.MetricSourceSbom,
		metrics.MetricSourceMemory,
		metrics.MetricSourceTCPQueueLength,
		metrics.MetricSourceOomKill,
		metrics.MetricSourceContainerLifecycle,
		metrics.MetricSourceJetson,
		metrics.MetricSourceContainerImage,
		metrics.MetricSourceCPU,
		metrics.MetricSourceLoad,
		metrics.MetricSourceDisk,
		metrics.MetricSourceNetwork,
		metrics.MetricSourceSnmp:
		return 11 // integration_metrics
	case metrics.MetricSourceOTLP,
		metrics.MetricSourceOTelActiveDirectoryDSReceiver,
		metrics.MetricSourceOTelAerospikeReceiver,
		metrics.MetricSourceOTelApacheReceiver,
		metrics.MetricSourceOTelApacheSparkReceiver,
		metrics.MetricSourceOTelAzureMonitorReceiver,
		metrics.MetricSourceOTelBigIPReceiver,
		metrics.MetricSourceOTelChronyReceiver,
		metrics.MetricSourceOTelCouchDBReceiver,
		metrics.MetricSourceOTelDockerStatsReceiver,
		metrics.MetricSourceOTelElasticsearchReceiver,
		metrics.MetricSourceOTelExpVarReceiver,
		metrics.MetricSourceOTelFileStatsReceiver,
		metrics.MetricSourceOTelFlinkMetricsReceiver,
		metrics.MetricSourceOTelGitProviderReceiver,
		metrics.MetricSourceOTelHAProxyReceiver,
		metrics.MetricSourceOTelHostMetricsReceiver,
		metrics.MetricSourceOTelHTTPCheckReceiver,
		metrics.MetricSourceOTelIISReceiver,
		metrics.MetricSourceOTelK8SClusterReceiver,
		metrics.MetricSourceOTelKafkaMetricsReceiver,
		metrics.MetricSourceOTelKubeletStatsReceiver,
		metrics.MetricSourceOTelMemcachedReceiver,
		metrics.MetricSourceOTelMongoDBAtlasReceiver,
		metrics.MetricSourceOTelMongoDBReceiver,
		metrics.MetricSourceOTelMySQLReceiver,
		metrics.MetricSourceOTelNginxReceiver,
		metrics.MetricSourceOTelNSXTReceiver,
		metrics.MetricSourceOTelOracleDBReceiver,
		metrics.MetricSourceOTelPostgreSQLReceiver,
		metrics.MetricSourceOTelPrometheusReceiver,
		metrics.MetricSourceOTelRabbitMQReceiver,
		metrics.MetricSourceOTelRedisReceiver,
		metrics.MetricSourceOTelRiakReceiver,
		metrics.MetricSourceOTelSAPHANAReceiver,
		metrics.MetricSourceOTelSNMPReceiver,
		metrics.MetricSourceOTelSnowflakeReceiver,
		metrics.MetricSourceOTelSplunkEnterpriseReceiver,
		metrics.MetricSourceOTelSQLServerReceiver,
		metrics.MetricSourceOTelSSHCheckReceiver,
		metrics.MetricSourceOTelStatsDReceiver,
		metrics.MetricSourceOTelVCenterReceiver,
		metrics.MetricSourceOTelZookeeperReceiver:
		return 0 // TODO: otlp
	default:
		return 0
	}
}

func MetricSourceToOriginService(ms metrics.MetricSource) int32 {
	// These constants map to specific fields in the 'OriginService' enum in origin.proto
	switch ms {
	case metrics.MetricSourceDogstatsd:
		return 0
	case metrics.MetricSourceJmxCustom:
		return 9
	case metrics.MetricSourceUnknown:
		return 0
	case metrics.MetricSourceActivemq:
		return 12
	case metrics.MetricSourceCassandra:
		return 28
	case metrics.MetricSourceConfluentPlatform:
		return 40
	case metrics.MetricSourceDisk:
		return 48
	case metrics.MetricSourceHazelcast:
		return 70
	case metrics.MetricSourceHive:
		return 73
	case metrics.MetricSourceHivemq:
		return 74
	case metrics.MetricSourceHudi:
		return 76
	case metrics.MetricSourceIgnite:
		return 83
	case metrics.MetricSourceJbossWildfly:
		return 87
	case metrics.MetricSourceKafka:
		return 90
	case metrics.MetricSourceNetwork:
		return 114
	case metrics.MetricSourcePresto:
		return 130
	case metrics.MetricSourceSnmp:
		return 145
	case metrics.MetricSourceSolr:
		return 147
	case metrics.MetricSourceSonarqube:
		return 148
	case metrics.MetricSourceTomcat:
		return 163
	case metrics.MetricSourceWeblogic:
		return 172
	case metrics.MetricSourceContainer:
		return 180
	case metrics.MetricSourceContainerd:
		return 181
	case metrics.MetricSourceCri:
		return 182
	case metrics.MetricSourceDocker:
		return 183
	case metrics.MetricSourceNtp:
		return 184
	case metrics.MetricSourceSystemd:
		return 185
	case metrics.MetricSourceHelm:
		return 186
	case metrics.MetricSourceKubernetesAPIServer:
		return 187
	case metrics.MetricSourceKubernetesStateCore:
		return 188
	case metrics.MetricSourceOrchestrator:
		return 189
	case metrics.MetricSourceWinproc:
		return 190
	case metrics.MetricSourceFileHandle:
		return 191
	case metrics.MetricSourceWinkmem:
		return 192
	case metrics.MetricSourceIo:
		return 193
	case metrics.MetricSourceUptime:
		return 194
	case metrics.MetricSourceSbom:
		return 195
	case metrics.MetricSourceMemory:
		return 196
	case metrics.MetricSourceTCPQueueLength:
		return 197
	case metrics.MetricSourceOomKill:
		return 198
	case metrics.MetricSourceContainerLifecycle:
		return 199
	case metrics.MetricSourceJetson:
		return 200
	case metrics.MetricSourceContainerImage:
		return 201
	case metrics.MetricSourceCPU:
		return 202
	case metrics.MetricSourceLoad:
		return 203
	case metrics.MetricSourceOTLP:
		return 0 // Nonspecific OTLP metric
	case metrics.MetricSourceOTelActiveDirectoryDSReceiver:
		return 0
	case metrics.MetricSourceOTelAerospikeReceiver:
		return 0
	case metrics.MetricSourceOTelApacheReceiver:
		return 0
	case metrics.MetricSourceOTelApacheSparkReceiver:
		return 0
	case metrics.MetricSourceOTelAzureMonitorReceiver:
		return 0
	case metrics.MetricSourceOTelBigIPReceiver:
		return 0
	case metrics.MetricSourceOTelChronyReceiver:
		return 0
	case metrics.MetricSourceOTelCouchDBReceiver:
		return 0
	case metrics.MetricSourceOTelDockerStatsReceiver:
		return 0
	case metrics.MetricSourceOTelElasticsearchReceiver:
		return 0
	case metrics.MetricSourceOTelExpVarReceiver:
		return 0
	case metrics.MetricSourceOTelFileStatsReceiver:
		return 0
	case metrics.MetricSourceOTelFlinkMetricsReceiver:
		return 0
	case metrics.MetricSourceOTelGitProviderReceiver:
		return 0
	case metrics.MetricSourceOTelHAProxyReceiver:
		return 0
	case metrics.MetricSourceOTelHostMetricsReceiver:
		return 0
	case metrics.MetricSourceOTelHTTPCheckReceiver:
		return 0
	case metrics.MetricSourceOTelIISReceiver:
		return 0
	case metrics.MetricSourceOTelK8SClusterReceiver:
		return 0
	case metrics.MetricSourceOTelKafkaMetricsReceiver:
		return 0
	case metrics.MetricSourceOTelKubeletStatsReceiver:
		return 0
	case metrics.MetricSourceOTelMemcachedReceiver:
		return 0
	case metrics.MetricSourceOTelMongoDBAtlasReceiver:
		return 0
	case metrics.MetricSourceOTelMongoDBReceiver:
		return 0
	case metrics.MetricSourceOTelMySQLReceiver:
		return 0
	case metrics.MetricSourceOTelNginxReceiver:
		return 0
	case metrics.MetricSourceOTelNSXTReceiver:
		return 0
	case metrics.MetricSourceOTelOracleDBReceiver:
		return 0
	case metrics.MetricSourceOTelPostgreSQLReceiver:
		return 0
	case metrics.MetricSourceOTelPrometheusReceiver:
		return 0
	case metrics.MetricSourceOTelRabbitMQReceiver:
		return 0
	case metrics.MetricSourceOTelRedisReceiver:
		return 0
	case metrics.MetricSourceOTelRiakReceiver:
		return 0
	case metrics.MetricSourceOTelSAPHANAReceiver:
		return 0
	case metrics.MetricSourceOTelSNMPReceiver:
		return 0
	case metrics.MetricSourceOTelSnowflakeReceiver:
		return 0
	case metrics.MetricSourceOTelSplunkEnterpriseReceiver:
		return 0
	case metrics.MetricSourceOTelSQLServerReceiver:
		return 0
	case metrics.MetricSourceOTelSSHCheckReceiver:
		return 0
	case metrics.MetricSourceOTelStatsDReceiver:
		return 0
	case metrics.MetricSourceOTelVCenterReceiver:
		return 0
	case metrics.MetricSourceOTelZookeeperReceiver:
		return 0
	default:
		return 0
	}

}
