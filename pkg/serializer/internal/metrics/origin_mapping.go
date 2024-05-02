// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func metricSourceToOriginCategory(ms metrics.MetricSource) int32 {
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
		// Core Checks
		metrics.MetricSourceInternal,
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
		metrics.MetricSourceSnmp,
		// Python Checks
		metrics.MetricSourceActiveDirectory,
		metrics.MetricSourceActivemqXML,
		metrics.MetricSourceAerospike,
		metrics.MetricSourceAgentMetrics,
		metrics.MetricSourceAirbyte,
		metrics.MetricSourceAirflow,
		metrics.MetricSourceAmazonEks,
		metrics.MetricSourceAmazonEksBlueprints,
		metrics.MetricSourceAmazonMsk,
		metrics.MetricSourceAmbari,
		metrics.MetricSourceApache,
		metrics.MetricSourceArangodb,
		metrics.MetricSourceArgoRollouts,
		metrics.MetricSourceArgoWorkflows,
		metrics.MetricSourceArgocd,
		metrics.MetricSourceAspdotnet,
		metrics.MetricSourceAviVantage,
		metrics.MetricSourceAzureActiveDirectory,
		metrics.MetricSourceAzureIotEdge,
		metrics.MetricSourceBoundary,
		metrics.MetricSourceBtrfs,
		metrics.MetricSourceCacti,
		metrics.MetricSourceCalico,
		metrics.MetricSourceCassandraNodetool,
		metrics.MetricSourceCeph,
		metrics.MetricSourceCertManager,
		metrics.MetricSourceCheckpointQuantumFirewall,
		metrics.MetricSourceCilium,
		metrics.MetricSourceCiscoAci,
		metrics.MetricSourceCiscoDuo,
		metrics.MetricSourceCiscoSecureFirewall,
		metrics.MetricSourceCiscoUmbrellaDNS,
		metrics.MetricSourceCitrixHypervisor,
		metrics.MetricSourceClickhouse,
		metrics.MetricSourceCloudFoundryAPI,
		metrics.MetricSourceCloudera,
		metrics.MetricSourceCockroachdb,
		metrics.MetricSourceConsul,
		metrics.MetricSourceConsulConnect,
		metrics.MetricSourceCoredns,
		metrics.MetricSourceCouch,
		metrics.MetricSourceCouchbase,
		metrics.MetricSourceCrio,
		metrics.MetricSourceDatabricks,
		metrics.MetricSourceDatadogChecksBase,
		metrics.MetricSourceDatadogChecksDependencyProvider,
		metrics.MetricSourceDatadogChecksDev,
		metrics.MetricSourceDatadogChecksDownloader,
		metrics.MetricSourceDatadogChecksTestsHelper,
		metrics.MetricSourceDatadogClusterAgent,
		metrics.MetricSourceDatadogOperator,
		metrics.MetricSourceDcgm,
		metrics.MetricSourceDdev,
		metrics.MetricSourceDirectory,
		metrics.MetricSourceDNSCheck,
		metrics.MetricSourceDockerDaemon,
		metrics.MetricSourceDotnetclr,
		metrics.MetricSourceDruid,
		metrics.MetricSourceEcsFargate,
		metrics.MetricSourceEksAnywhere,
		metrics.MetricSourceEksFargate,
		metrics.MetricSourceElastic,
		metrics.MetricSourceEnvoy,
		metrics.MetricSourceEtcd,
		metrics.MetricSourceExchangeServer,
		metrics.MetricSourceExternalDNS,
		metrics.MetricSourceFlink,
		metrics.MetricSourceFluentd,
		metrics.MetricSourceFluxcd,
		metrics.MetricSourceFoundationdb,
		metrics.MetricSourceGearmand,
		metrics.MetricSourceGitlab,
		metrics.MetricSourceGitlabRunner,
		metrics.MetricSourceGke,
		metrics.MetricSourceGlusterfs,
		metrics.MetricSourceGoMetro,
		metrics.MetricSourceGoExpvar,
		metrics.MetricSourceGunicorn,
		metrics.MetricSourceHaproxy,
		metrics.MetricSourceHarbor,
		metrics.MetricSourceHdfsDatanode,
		metrics.MetricSourceHdfsNamenode,
		metrics.MetricSourceHTTPCheck,
		metrics.MetricSourceHyperv,
		metrics.MetricSourceIamAccessAnalyzer,
		metrics.MetricSourceIbmAce,
		metrics.MetricSourceIbmDb2,
		metrics.MetricSourceIbmI,
		metrics.MetricSourceIbmMq,
		metrics.MetricSourceIbmWas,
		metrics.MetricSourceIis,
		metrics.MetricSourceImpala,
		metrics.MetricSourceIstio,
		metrics.MetricSourceJmeter,
		metrics.MetricSourceJournald,
		metrics.MetricSourceKafkaConsumer,
		metrics.MetricSourceKarpenter,
		metrics.MetricSourceKong,
		metrics.MetricSourceKubeAPIserverMetrics,
		metrics.MetricSourceKubeControllerManager,
		metrics.MetricSourceKubeDNS,
		metrics.MetricSourceKubeMetricsServer,
		metrics.MetricSourceKubeProxy,
		metrics.MetricSourceKubeScheduler,
		metrics.MetricSourceKubelet,
		metrics.MetricSourceKubernetes,
		metrics.MetricSourceKubernetesState,
		metrics.MetricSourceKyototycoon,
		metrics.MetricSourceLangchain,
		metrics.MetricSourceLighttpd,
		metrics.MetricSourceLinkerd,
		metrics.MetricSourceLinuxProcExtras,
		metrics.MetricSourceMapr,
		metrics.MetricSourceMapreduce,
		metrics.MetricSourceMarathon,
		metrics.MetricSourceMarklogic,
		metrics.MetricSourceMcache,
		metrics.MetricSourceMesosMaster,
		metrics.MetricSourceMesosSlave,
		metrics.MetricSourceMongo,
		metrics.MetricSourceMysql,
		metrics.MetricSourceNagios,
		metrics.MetricSourceNfsstat,
		metrics.MetricSourceNginx,
		metrics.MetricSourceNginxIngressController,
		metrics.MetricSourceNvidiaJetson,
		metrics.MetricSourceNvidiaTriton,
		metrics.MetricSourceOke,
		metrics.MetricSourceOpenai,
		metrics.MetricSourceOpenldap,
		metrics.MetricSourceOpenmetrics,
		metrics.MetricSourceOpenshift,
		metrics.MetricSourceOpenstack,
		metrics.MetricSourceOpenstackController,
		metrics.MetricSourceOracle,
		metrics.MetricSourceOtel,
		metrics.MetricSourcePaloAltoPanorama,
		metrics.MetricSourcePanFirewall,
		metrics.MetricSourcePdhCheck,
		metrics.MetricSourcePgbouncer,
		metrics.MetricSourcePhpFpm,
		metrics.MetricSourcePivotalPks,
		metrics.MetricSourcePodman,
		metrics.MetricSourcePostfix,
		metrics.MetricSourcePostgres,
		metrics.MetricSourcePowerdnsRecursor,
		metrics.MetricSourceProcess,
		metrics.MetricSourcePrometheus,
		metrics.MetricSourceProxysql,
		metrics.MetricSourcePulsar,
		metrics.MetricSourceRabbitmq,
		metrics.MetricSourceRay,
		metrics.MetricSourceRedisdb,
		metrics.MetricSourceRethinkdb,
		metrics.MetricSourceRiak,
		metrics.MetricSourceRiakcs,
		metrics.MetricSourceSapHana,
		metrics.MetricSourceScylla,
		metrics.MetricSourceSidekiq,
		metrics.MetricSourceSilk,
		metrics.MetricSourceSinglestore,
		metrics.MetricSourceSnmpAmericanPowerConversion,
		metrics.MetricSourceSnmpArista,
		metrics.MetricSourceSnmpAruba,
		metrics.MetricSourceSnmpChatsworthProducts,
		metrics.MetricSourceSnmpCheckPoint,
		metrics.MetricSourceSnmpCisco,
		metrics.MetricSourceSnmpDell,
		metrics.MetricSourceSnmpF5,
		metrics.MetricSourceSnmpFortinet,
		metrics.MetricSourceSnmpHewlettPackardEnterprise,
		metrics.MetricSourceSnmpJuniper,
		metrics.MetricSourceSnmpNetapp,
		metrics.MetricSourceSnowflake,
		metrics.MetricSourceSpark,
		metrics.MetricSourceSqlserver,
		metrics.MetricSourceSquid,
		metrics.MetricSourceSSHCheck,
		metrics.MetricSourceStatsd,
		metrics.MetricSourceStrimzi,
		metrics.MetricSourceSupervisord,
		metrics.MetricSourceSystemCore,
		metrics.MetricSourceSystemSwap,
		metrics.MetricSourceTCPCheck,
		metrics.MetricSourceTeamcity,
		metrics.MetricSourceTekton,
		metrics.MetricSourceTemporal,
		metrics.MetricSourceTenable,
		metrics.MetricSourceTeradata,
		metrics.MetricSourceTerraform,
		metrics.MetricSourceTLS,
		metrics.MetricSourceTokumx,
		metrics.MetricSourceTorchserve,
		metrics.MetricSourceTrafficServer,
		metrics.MetricSourceTwemproxy,
		metrics.MetricSourceTwistlock,
		metrics.MetricSourceVarnish,
		metrics.MetricSourceVault,
		metrics.MetricSourceVertica,
		metrics.MetricSourceVoltdb,
		metrics.MetricSourceVsphere,
		metrics.MetricSourceWeaviate,
		metrics.MetricSourceWin32EventLog,
		metrics.MetricSourceWincrashdetect,
		metrics.MetricSourceWindowsPerformanceCounters,
		metrics.MetricSourceWindowsRegistry,
		metrics.MetricSourceWindowsService,
		metrics.MetricSourceWmiCheck,
		metrics.MetricSourceYarn,
		metrics.MetricSourceZeek,
		metrics.MetricSourceZk:
		return 11 // integrationMetrics
	default:
		return 0
	}
}

func metricSourceToOriginService(ms metrics.MetricSource) int32 {
	// These constants map to specific fields in the 'OriginService' enum in origin.proto
	switch ms {
	case metrics.MetricSourceDogstatsd:
		return 0
	case metrics.MetricSourceJmxCustom:
		return 9
	case metrics.MetricSourceUnknown:
		return 0
	case metrics.MetricSourceActiveDirectory:
		return 10
	case metrics.MetricSourceActivemq:
		return 12
	case metrics.MetricSourceAerospike:
		return 13
	case metrics.MetricSourceAirflow:
		return 14
	case metrics.MetricSourceAmbari:
		return 16
	case metrics.MetricSourceApache:
		return 17
	case metrics.MetricSourceArangodb:
		return 18
	case metrics.MetricSourceArgocd:
		return 19
	case metrics.MetricSourceAspdotnet:
		return 20
	case metrics.MetricSourceAviVantage:
		return 21
	case metrics.MetricSourceAzureIotEdge:
		return 22
	case metrics.MetricSourceBoundary:
		return 23
	case metrics.MetricSourceBtrfs:
		return 24
	case metrics.MetricSourceCacti:
		return 25
	case metrics.MetricSourceCalico:
		return 26
	case metrics.MetricSourceCassandraNodetool:
		return 27
	case metrics.MetricSourceCassandra:
		return 28
	case metrics.MetricSourceCeph:
		return 29
	case metrics.MetricSourceCertManager:
		return 30
	case metrics.MetricSourceCilium:
		return 34
	case metrics.MetricSourceCiscoAci:
		return 35
	case metrics.MetricSourceCitrixHypervisor:
		return 36
	case metrics.MetricSourceClickhouse:
		return 37
	case metrics.MetricSourceCloudFoundryAPI:
		return 38
	case metrics.MetricSourceCockroachdb:
		return 39
	case metrics.MetricSourceConfluentPlatform:
		return 40
	case metrics.MetricSourceConsul:
		return 41
	case metrics.MetricSourceCoredns:
		return 42
	case metrics.MetricSourceCouch:
		return 43
	case metrics.MetricSourceCouchbase:
		return 44
	case metrics.MetricSourceCrio:
		return 45
	case metrics.MetricSourceDirectory:
		return 47
	case metrics.MetricSourceDisk:
		return 48
	case metrics.MetricSourceDNSCheck:
		return 49
	case metrics.MetricSourceDotnetclr:
		return 50
	case metrics.MetricSourceDruid:
		return 51
	case metrics.MetricSourceEcsFargate:
		return 52
	case metrics.MetricSourceEksFargate:
		return 53
	case metrics.MetricSourceElastic:
		return 54
	case metrics.MetricSourceEnvoy:
		return 55
	case metrics.MetricSourceEtcd:
		return 56
	case metrics.MetricSourceExchangeServer:
		return 57
	case metrics.MetricSourceExternalDNS:
		return 58
	case metrics.MetricSourceFlink:
		return 59
	case metrics.MetricSourceFluentd:
		return 60
	case metrics.MetricSourceFoundationdb:
		return 61
	case metrics.MetricSourceGearmand:
		return 62
	case metrics.MetricSourceGitlabRunner:
		return 63
	case metrics.MetricSourceGitlab:
		return 64
	case metrics.MetricSourceGlusterfs:
		return 65
	case metrics.MetricSourceGoExpvar:
		return 66
	case metrics.MetricSourceGunicorn:
		return 67
	case metrics.MetricSourceHaproxy:
		return 68
	case metrics.MetricSourceHarbor:
		return 69
	case metrics.MetricSourceHazelcast:
		return 70
	case metrics.MetricSourceHdfsDatanode:
		return 71
	case metrics.MetricSourceHdfsNamenode:
		return 72
	case metrics.MetricSourceHive:
		return 73
	case metrics.MetricSourceHivemq:
		return 74
	case metrics.MetricSourceHTTPCheck:
		return 75
	case metrics.MetricSourceHudi:
		return 76
	case metrics.MetricSourceHyperv:
		return 77
	case metrics.MetricSourceIbmAce:
		return 78
	case metrics.MetricSourceIbmDb2:
		return 79
	case metrics.MetricSourceIbmI:
		return 80
	case metrics.MetricSourceIbmMq:
		return 81
	case metrics.MetricSourceIbmWas:
		return 82
	case metrics.MetricSourceIgnite:
		return 83
	case metrics.MetricSourceIis:
		return 84
	case metrics.MetricSourceImpala:
		return 85
	case metrics.MetricSourceIstio:
		return 86
	case metrics.MetricSourceJbossWildfly:
		return 87
	case metrics.MetricSourceKafkaConsumer:
		return 89
	case metrics.MetricSourceKafka:
		return 90
	case metrics.MetricSourceKong:
		return 91
	case metrics.MetricSourceKubeAPIserverMetrics:
		return 92
	case metrics.MetricSourceKubeControllerManager:
		return 93
	case metrics.MetricSourceKubeDNS:
		return 94
	case metrics.MetricSourceKubeMetricsServer:
		return 95
	case metrics.MetricSourceKubeProxy:
		return 96
	case metrics.MetricSourceKubeScheduler:
		return 97
	case metrics.MetricSourceKubelet:
		return 98
	case metrics.MetricSourceKubernetesState:
		return 99
	case metrics.MetricSourceKyototycoon:
		return 100
	case metrics.MetricSourceLighttpd:
		return 101
	case metrics.MetricSourceLinkerd:
		return 102
	case metrics.MetricSourceLinuxProcExtras:
		return 103
	case metrics.MetricSourceMapr:
		return 104
	case metrics.MetricSourceMapreduce:
		return 105
	case metrics.MetricSourceMarathon:
		return 106
	case metrics.MetricSourceMarklogic:
		return 107
	case metrics.MetricSourceMcache:
		return 108
	case metrics.MetricSourceMesosMaster:
		return 109
	case metrics.MetricSourceMesosSlave:
		return 110
	case metrics.MetricSourceMongo:
		return 111
	case metrics.MetricSourceMysql:
		return 112
	case metrics.MetricSourceNagios:
		return 113
	case metrics.MetricSourceNetwork:
		return 114
	case metrics.MetricSourceNfsstat:
		return 115
	case metrics.MetricSourceNginxIngressController:
		return 116
	case metrics.MetricSourceNginx:
		return 117
	case metrics.MetricSourceOpenldap:
		return 118
	case metrics.MetricSourceOpenmetrics:
		return 119
	case metrics.MetricSourceOpenstackController:
		return 120
	case metrics.MetricSourceOpenstack:
		return 121
	case metrics.MetricSourceOracle:
		return 122
	case metrics.MetricSourcePdhCheck:
		return 124
	case metrics.MetricSourcePgbouncer:
		return 125
	case metrics.MetricSourcePhpFpm:
		return 126
	case metrics.MetricSourcePostfix:
		return 127
	case metrics.MetricSourcePostgres:
		return 128
	case metrics.MetricSourcePowerdnsRecursor:
		return 129
	case metrics.MetricSourcePresto:
		return 130
	case metrics.MetricSourceProcess:
		return 131
	case metrics.MetricSourcePrometheus:
		return 132
	case metrics.MetricSourceProxysql:
		return 133
	case metrics.MetricSourcePulsar:
		return 134
	case metrics.MetricSourceRabbitmq:
		return 135
	case metrics.MetricSourceRedisdb:
		return 136
	case metrics.MetricSourceRethinkdb:
		return 137
	case metrics.MetricSourceRiak:
		return 138
	case metrics.MetricSourceRiakcs:
		return 139
	case metrics.MetricSourceSapHana:
		return 140
	case metrics.MetricSourceScylla:
		return 141
	case metrics.MetricSourceSidekiq:
		return 142
	case metrics.MetricSourceSilk:
		return 143
	case metrics.MetricSourceSinglestore:
		return 144
	case metrics.MetricSourceSnmp:
		return 145
	case metrics.MetricSourceSnowflake:
		return 146
	case metrics.MetricSourceSolr:
		return 147
	case metrics.MetricSourceSonarqube:
		return 148
	case metrics.MetricSourceSpark:
		return 149
	case metrics.MetricSourceSqlserver:
		return 150
	case metrics.MetricSourceSquid:
		return 151
	case metrics.MetricSourceSSHCheck:
		return 152
	case metrics.MetricSourceStatsd:
		return 153
	case metrics.MetricSourceSupervisord:
		return 154
	case metrics.MetricSourceSystemCore:
		return 155
	case metrics.MetricSourceSystemSwap:
		return 156
	case metrics.MetricSourceTCPCheck:
		return 157
	case metrics.MetricSourceTeamcity:
		return 158
	case metrics.MetricSourceTeradata:
		return 160
	case metrics.MetricSourceTLS:
		return 161
	case metrics.MetricSourceTokumx:
		return 162
	case metrics.MetricSourceTomcat:
		return 163
	case metrics.MetricSourceTrafficServer:
		return 164
	case metrics.MetricSourceTwemproxy:
		return 165
	case metrics.MetricSourceTwistlock:
		return 166
	case metrics.MetricSourceVarnish:
		return 167
	case metrics.MetricSourceVault:
		return 168
	case metrics.MetricSourceVertica:
		return 169
	case metrics.MetricSourceVoltdb:
		return 170
	case metrics.MetricSourceVsphere:
		return 171
	case metrics.MetricSourceWeblogic:
		return 172
	case metrics.MetricSourceWin32EventLog:
		return 173
	case metrics.MetricSourceWindowsPerformanceCounters:
		return 174
	case metrics.MetricSourceWindowsService:
		return 175
	case metrics.MetricSourceWmiCheck:
		return 176
	case metrics.MetricSourceYarn:
		return 177
	case metrics.MetricSourceZk:
		return 178
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
	case metrics.MetricSourceInternal:
		return 212
	default:
		return 0
	}

}
