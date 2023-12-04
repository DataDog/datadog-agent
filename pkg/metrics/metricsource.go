// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// MetricSource represents how this metric made it into the Agent
type MetricSource uint16

// Enumeration of the currently supported MetricSources
const (
	MetricSourceUnknown MetricSource = iota
	MetricSourceDogstatsd

	// JMX Integrations
	MetricSourceJmxCustom
	MetricSourceActivemq
	MetricSourceCassandra
	MetricSourceConfluentPlatform
	MetricSourceHazelcast
	MetricSourceHive
	MetricSourceHivemq
	MetricSourceHudi
	MetricSourceIgnite
	MetricSourceJbossWildfly
	MetricSourceKafka
	MetricSourcePresto
	MetricSourceSolr
	MetricSourceSonarqube
	MetricSourceTomcat
	MetricSourceWeblogic

	// Core Checks
	MetricSourceContainer
	MetricSourceContainerd
	MetricSourceCri
	MetricSourceDocker
	MetricSourceNtp
	MetricSourceSystemd
	MetricSourceHelm
	MetricSourceKubernetesAPIServer
	MetricSourceKubernetesStateCore
	MetricSourceOrchestrator
	MetricSourceWinproc
	MetricSourceFileHandle
	MetricSourceWinkmem
	MetricSourceIo
	MetricSourceUptime
	MetricSourceSbom
	MetricSourceMemory
	MetricSourceTCPQueueLength
	MetricSourceOomKill
	MetricSourceContainerLifecycle
	MetricSourceJetson
	MetricSourceContainerImage
	MetricSourceCPU
	MetricSourceLoad
	MetricSourceDisk
	MetricSourceNetwork
	MetricSourceSnmp

	// OpenTelemetry
	MetricSourceOTLP
	MetricSourceOTelActiveDirectoryDSReceiver
	MetricSourceOTelAerospikeReceiver
	MetricSourceOTelApacheReceiver
	MetricSourceOTelApacheSparkReceiver
	MetricSourceOTelAzureMonitorReceiver
	MetricSourceOTelBigIPReceiver
	MetricSourceOTelChronyReceiver
	MetricSourceOTelCouchDBReceiver
	MetricSourceOTelDockerStatsReceiver
	MetricSourceOTelElasticsearchReceiver
	MetricSourceOTelExpVarReceiver
	MetricSourceOTelFileStatsReceiver
	MetricSourceOTelFlinkMetricsReceiver
	MetricSourceOTelGitProviderReceiver
	MetricSourceOTelHAProxyReceiver
	MetricSourceOTelHostMetricsReceiver
	MetricSourceOTelHTTPCheckReceiver
	MetricSourceOTelIISReceiver
	MetricSourceOTelK8SClusterReceiver
	MetricSourceOTelKafkaMetricsReceiver
	MetricSourceOTelKubeletStatsReceiver
	MetricSourceOTelMemcachedReceiver
	MetricSourceOTelMongoDBAtlasReceiver
	MetricSourceOTelMongoDBReceiver
	MetricSourceOTelMySQLReceiver
	MetricSourceOTelNginxReceiver
	MetricSourceOTelNSXTReceiver
	MetricSourceOTelOracleDBReceiver
	MetricSourceOTelPostgreSQLReceiver
	MetricSourceOTelPrometheusReceiver
	MetricSourceOTelRabbitMQReceiver
	MetricSourceOTelRedisReceiver
	MetricSourceOTelRiakReceiver
	MetricSourceOTelSAPHANAReceiver
	MetricSourceOTelSNMPReceiver
	MetricSourceOTelSnowflakeReceiver
	MetricSourceOTelSplunkEnterpriseReceiver
	MetricSourceOTelSQLServerReceiver
	MetricSourceOTelSSHCheckReceiver
	MetricSourceOTelStatsDReceiver
	MetricSourceOTelVCenterReceiver
	MetricSourceOTelZookeeperReceiver
)

// String returns a string representation of MetricSource
func (ms MetricSource) String() string {
	switch ms {
	case MetricSourceDogstatsd:
		return "dogstatsd"
	case MetricSourceJmxCustom:
		return "jmx-custom-check"
	case MetricSourceActivemq:
		return "activemq"
	case MetricSourceCassandra:
		return "cassandra"
	case MetricSourceConfluentPlatform:
		return "confluent_platform"
	case MetricSourceHazelcast:
		return "hazelcast"
	case MetricSourceHive:
		return "hive"
	case MetricSourceHivemq:
		return "hivemq"
	case MetricSourceHudi:
		return "hudi"
	case MetricSourceIgnite:
		return "ignite"
	case MetricSourceJbossWildfly:
		return "jboss_wildfly"
	case MetricSourceKafka:
		return "kafka"
	case MetricSourcePresto:
		return "presto"
	case MetricSourceSolr:
		return "solr"
	case MetricSourceSonarqube:
		return "sonarqube"
	case MetricSourceTomcat:
		return "tomcat"
	case MetricSourceWeblogic:
		return "weblogic"
	case MetricSourceContainer:
		return "container"
	case MetricSourceContainerd:
		return "containerd"
	case MetricSourceCri:
		return "cri"
	case MetricSourceDocker:
		return "docker"
	case MetricSourceNtp:
		return "ntp"
	case MetricSourceSystemd:
		return "systemd"
	case MetricSourceHelm:
		return "helm"
	case MetricSourceKubernetesAPIServer:
		return "kubernetes_apiserver"
	case MetricSourceKubernetesStateCore:
		return "kubernetes_state_core"
	case MetricSourceOrchestrator:
		return "orchestrator"
	case MetricSourceWinproc:
		return "winproc"
	case MetricSourceFileHandle:
		return "file_handle"
	case MetricSourceWinkmem:
		return "winkmem"
	case MetricSourceIo:
		return "io"
	case MetricSourceUptime:
		return "uptime"
	case MetricSourceSbom:
		return "sbom"
	case MetricSourceMemory:
		return "memory"
	case MetricSourceTCPQueueLength:
		return "tcp_queue_length"
	case MetricSourceOomKill:
		return "oom_kill"
	case MetricSourceContainerLifecycle:
		return "container_lifecycle"
	case MetricSourceJetson:
		return "jetson"
	case MetricSourceContainerImage:
		return "container_image"
	case MetricSourceCPU:
		return "cpu"
	case MetricSourceLoad:
		return "load"
	case MetricSourceDisk:
		return "disk"
	case MetricSourceNetwork:
		return "network"
	case MetricSourceSnmp:
		return "snmp"
	case MetricSourceOTLP:
		return "otlp"
	case MetricSourceOTelActiveDirectoryDSReceiver:
		return "otlp-activedirectoryds"
	case MetricSourceOTelAerospikeReceiver:
		return "otlp-aerospike"
	case MetricSourceOTelApacheReceiver:
		return "otlp-apache"
	case MetricSourceOTelApacheSparkReceiver:
		return "otlp-apache-spark"
	case MetricSourceOTelAzureMonitorReceiver:
		return "otlp-azure-monitor"
	case MetricSourceOTelBigIPReceiver:
		return "otlp-bigip"
	case MetricSourceOTelChronyReceiver:
		return "otlp-chrony"
	case MetricSourceOTelCouchDBReceiver:
		return "otlp-couchdb"
	case MetricSourceOTelDockerStatsReceiver:
		return "otlp-docker-stats"
	case MetricSourceOTelElasticsearchReceiver:
		return "otlp-elasticsearch"
	case MetricSourceOTelExpVarReceiver:
		return "otlp-expvar"
	case MetricSourceOTelFileStatsReceiver:
		return "otlp-file-stats"
	case MetricSourceOTelFlinkMetricsReceiver:
		return "otlp-flink-metrics"
	case MetricSourceOTelGitProviderReceiver:
		return "otlp-git-provider"
	case MetricSourceOTelHAProxyReceiver:
		return "otlp-haproxy"
	case MetricSourceOTelHostMetricsReceiver:
		return "otlp-host-metrics"
	case MetricSourceOTelHTTPCheckReceiver:
		return "otlp-http-check"
	case MetricSourceOTelIISReceiver:
		return "otlp-iis"
	case MetricSourceOTelK8SClusterReceiver:
		return "otlp-k8s-cluster"
	case MetricSourceOTelKafkaMetricsReceiver:
		return "otlp-kafka-metrics"
	case MetricSourceOTelKubeletStatsReceiver:
		return "otlp-kubelet-stats"
	case MetricSourceOTelMemcachedReceiver:
		return "otlp-memcached"
	case MetricSourceOTelMongoDBAtlasReceiver:
		return "otlp-mongodb-atlas"
	case MetricSourceOTelMongoDBReceiver:
		return "otlp-mongodb"
	case MetricSourceOTelMySQLReceiver:
		return "otlp-mysql"
	case MetricSourceOTelNginxReceiver:
		return "otlp-nginx"
	case MetricSourceOTelNSXTReceiver:
		return "otlp-nsxt"
	case MetricSourceOTelOracleDBReceiver:
		return "otlp-oracle-db"
	case MetricSourceOTelPostgreSQLReceiver:
		return "otlp-postgresql"
	case MetricSourceOTelPrometheusReceiver:
		return "otlp-prometheus"
	case MetricSourceOTelRabbitMQReceiver:
		return "otlp-rabbitmq"
	case MetricSourceOTelRedisReceiver:
		return "otlp-redis"
	case MetricSourceOTelRiakReceiver:
		return "otlp-riak"
	case MetricSourceOTelSAPHANAReceiver:
		return "otlp-sap-hana"
	case MetricSourceOTelSNMPReceiver:
		return "otlp-snmp"
	case MetricSourceOTelSnowflakeReceiver:
		return "otlp-snowflake"
	case MetricSourceOTelSplunkEnterpriseReceiver:
		return "otlp-splunk-enterprise"
	case MetricSourceOTelSQLServerReceiver:
		return "otlp-sql-server"
	case MetricSourceOTelSSHCheckReceiver:
		return "otlp-ssh-check"
	case MetricSourceOTelStatsDReceiver:
		return "otlp-statsd"
	case MetricSourceOTelVCenterReceiver:
		return "otlp-vcenter"
	case MetricSourceOTelZookeeperReceiver:
		return "otlp-zookeeper"
	default:
		return "<unknown>"

	}
}

// CoreCheckToMetricSource returns a MetricSource given the name
func CoreCheckToMetricSource(name string) MetricSource {
	switch name {
	case "container":
		return MetricSourceContainer
	case "containerd":
		return MetricSourceContainerd
	case "cri":
		return MetricSourceCri
	case "docker":
		return MetricSourceDocker
	case "ntp":
		return MetricSourceNtp
	case "systemd":
		return MetricSourceSystemd
	case "helm":
		return MetricSourceHelm
	case "kubernetes_apiserver":
		return MetricSourceKubernetesAPIServer
	case "kubernetes_state_core":
		return MetricSourceKubernetesStateCore
	case "orchestrator":
		return MetricSourceOrchestrator
	case "winproc":
		return MetricSourceWinproc
	case "file_handle":
		return MetricSourceFileHandle
	case "winkmem":
		return MetricSourceWinkmem
	case "io":
		return MetricSourceIo
	case "uptime":
		return MetricSourceUptime
	case "sbom":
		return MetricSourceSbom
	case "memory":
		return MetricSourceMemory
	case "tcp_queue_length":
		return MetricSourceTCPQueueLength
	case "oom_kill":
		return MetricSourceOomKill
	case "container_lifecycle":
		return MetricSourceContainerLifecycle
	case "jetson":
		return MetricSourceJetson
	case "container_image":
		return MetricSourceContainerImage
	case "cpu":
		return MetricSourceCPU
	case "load":
		return MetricSourceLoad
	case "disk":
		return MetricSourceDisk
	case "network":
		return MetricSourceNetwork
	case "snmp":
		return MetricSourceSnmp
	default:
		return MetricSourceUnknown
	}
}

// JMXCheckNameToMetricSource returns a MetricSource given the checkName
func JMXCheckNameToMetricSource(name string) MetricSource {
	switch name {
	case "activemq":
		return MetricSourceActivemq
	case "cassandra":
		return MetricSourceCassandra
	case "confluent_platform":
		return MetricSourceConfluentPlatform
	case "hazelcast":
		return MetricSourceHazelcast
	case "hive":
		return MetricSourceHive
	case "hivemq":
		return MetricSourceHivemq
	case "hudi":
		return MetricSourceHudi
	case "ignite":
		return MetricSourceIgnite
	case "jboss_wildfly":
		return MetricSourceJbossWildfly
	case "kafka":
		return MetricSourceKafka
	case "presto":
		return MetricSourcePresto
	case "solr":
		return MetricSourceSolr
	case "sonarqube":
		return MetricSourceSonarqube
	case "tomcat":
		return MetricSourceTomcat
	case "weblogic":
		return MetricSourceWeblogic
	default:
		return MetricSourceJmxCustom
	}
}
