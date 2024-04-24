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
	MetricSourceInternal
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

	// Python Checks
	MetricSourceActiveDirectory
	MetricSourceActivemqXML
	MetricSourceAerospike
	MetricSourceAgentMetrics
	MetricSourceAirbyte
	MetricSourceAirflow
	MetricSourceAmazonEks
	MetricSourceAmazonEksBlueprints
	MetricSourceAmazonMsk
	MetricSourceAmbari
	MetricSourceApache
	MetricSourceArangodb
	MetricSourceArgoRollouts
	MetricSourceArgoWorkflows
	MetricSourceArgocd
	MetricSourceAspdotnet
	MetricSourceAviVantage
	MetricSourceAzureActiveDirectory
	MetricSourceAzureIotEdge
	MetricSourceBoundary
	MetricSourceBtrfs
	MetricSourceCacti
	MetricSourceCalico
	MetricSourceCassandraNodetool
	MetricSourceCeph
	MetricSourceCertManager
	MetricSourceCheckpointQuantumFirewall
	MetricSourceCilium
	MetricSourceCiscoAci
	MetricSourceCiscoDuo
	MetricSourceCiscoSecureFirewall
	MetricSourceCiscoUmbrellaDNS
	MetricSourceCitrixHypervisor
	MetricSourceClickhouse
	MetricSourceCloudFoundryAPI
	MetricSourceCloudera
	MetricSourceCockroachdb
	MetricSourceConsul
	MetricSourceConsulConnect
	MetricSourceCoredns
	MetricSourceCouch
	MetricSourceCouchbase
	MetricSourceCrio
	MetricSourceDatabricks
	MetricSourceDatadogChecksBase
	MetricSourceDatadogChecksDependencyProvider
	MetricSourceDatadogChecksDev
	MetricSourceDatadogChecksDownloader
	MetricSourceDatadogChecksTestsHelper
	MetricSourceDatadogClusterAgent
	MetricSourceDatadogOperator
	MetricSourceDcgm
	MetricSourceDdev
	MetricSourceDirectory
	MetricSourceDNSCheck
	MetricSourceDockerDaemon
	MetricSourceDotnetclr
	MetricSourceDruid
	MetricSourceEcsFargate
	MetricSourceEksAnywhere
	MetricSourceEksFargate
	MetricSourceElastic
	MetricSourceEnvoy
	MetricSourceEtcd
	MetricSourceExchangeServer
	MetricSourceExternalDNS
	MetricSourceFlink
	MetricSourceFluentd
	MetricSourceFluxcd
	MetricSourceFoundationdb
	MetricSourceGearmand
	MetricSourceGitlab
	MetricSourceGitlabRunner
	MetricSourceGke
	MetricSourceGlusterfs
	MetricSourceGoMetro
	MetricSourceGoExpvar
	MetricSourceGunicorn
	MetricSourceHaproxy
	MetricSourceHarbor
	MetricSourceHdfsDatanode
	MetricSourceHdfsNamenode
	MetricSourceHTTPCheck
	MetricSourceHyperv
	MetricSourceIamAccessAnalyzer
	MetricSourceIbmAce
	MetricSourceIbmDb2
	MetricSourceIbmI
	MetricSourceIbmMq
	MetricSourceIbmWas
	MetricSourceIis
	MetricSourceImpala
	MetricSourceIstio
	MetricSourceJmeter
	MetricSourceJournald
	MetricSourceKafkaConsumer
	MetricSourceKarpenter
	MetricSourceKong
	MetricSourceKubeAPIserverMetrics
	MetricSourceKubeControllerManager
	MetricSourceKubeDNS
	MetricSourceKubeMetricsServer
	MetricSourceKubeProxy
	MetricSourceKubeScheduler
	MetricSourceKubelet
	MetricSourceKubernetes
	MetricSourceKubernetesState
	MetricSourceKyototycoon
	MetricSourceLangchain
	MetricSourceLighttpd
	MetricSourceLinkerd
	MetricSourceLinuxProcExtras
	MetricSourceMapr
	MetricSourceMapreduce
	MetricSourceMarathon
	MetricSourceMarklogic
	MetricSourceMcache
	MetricSourceMesosMaster
	MetricSourceMesosSlave
	MetricSourceMongo
	MetricSourceMysql
	MetricSourceNagios
	MetricSourceNfsstat
	MetricSourceNginx
	MetricSourceNginxIngressController
	MetricSourceNvidiaJetson
	MetricSourceNvidiaTriton
	MetricSourceOke
	MetricSourceOpenai
	MetricSourceOpenldap
	MetricSourceOpenmetrics
	MetricSourceOpenshift
	MetricSourceOpenstack
	MetricSourceOpenstackController
	MetricSourceOracle
	MetricSourceOtel
	MetricSourcePaloAltoPanorama
	MetricSourcePanFirewall
	MetricSourcePdhCheck
	MetricSourcePgbouncer
	MetricSourcePhpFpm
	MetricSourcePivotalPks
	MetricSourcePodman
	MetricSourcePostfix
	MetricSourcePostgres
	MetricSourcePowerdnsRecursor
	MetricSourceProcess
	MetricSourcePrometheus
	MetricSourceProxysql
	MetricSourcePulsar
	MetricSourceRabbitmq
	MetricSourceRay
	MetricSourceRedisdb
	MetricSourceRethinkdb
	MetricSourceRiak
	MetricSourceRiakcs
	MetricSourceSapHana
	MetricSourceScylla
	MetricSourceSidekiq
	MetricSourceSilk
	MetricSourceSinglestore
	MetricSourceSnmpAmericanPowerConversion
	MetricSourceSnmpArista
	MetricSourceSnmpAruba
	MetricSourceSnmpChatsworthProducts
	MetricSourceSnmpCheckPoint
	MetricSourceSnmpCisco
	MetricSourceSnmpDell
	MetricSourceSnmpF5
	MetricSourceSnmpFortinet
	MetricSourceSnmpHewlettPackardEnterprise
	MetricSourceSnmpJuniper
	MetricSourceSnmpNetapp
	MetricSourceSnowflake
	MetricSourceSpark
	MetricSourceSqlserver
	MetricSourceSquid
	MetricSourceSSHCheck
	MetricSourceStatsd
	MetricSourceStrimzi
	MetricSourceSupervisord
	MetricSourceSystemCore
	MetricSourceSystemSwap
	MetricSourceTCPCheck
	MetricSourceTeamcity
	MetricSourceTekton
	MetricSourceTemporal
	MetricSourceTenable
	MetricSourceTeradata
	MetricSourceTerraform
	MetricSourceTLS
	MetricSourceTokumx
	MetricSourceTorchserve
	MetricSourceTrafficServer
	MetricSourceTwemproxy
	MetricSourceTwistlock
	MetricSourceVarnish
	MetricSourceVault
	MetricSourceVertica
	MetricSourceVoltdb
	MetricSourceVsphere
	MetricSourceWeaviate
	MetricSourceWin32EventLog
	MetricSourceWincrashdetect
	MetricSourceWindowsPerformanceCounters
	MetricSourceWindowsRegistry
	MetricSourceWindowsService
	MetricSourceWmiCheck
	MetricSourceYarn
	MetricSourceZeek
	MetricSourceZk
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
	case MetricSourceInternal:
		return "internal"
	case MetricSourceActiveDirectory:
		return "active_directory"
	case MetricSourceActivemqXML:
		return "activemq_xml"
	case MetricSourceAerospike:
		return "aerospike"
	case MetricSourceAgentMetrics:
		return "agent_metrics"
	case MetricSourceAirbyte:
		return "airbyte"
	case MetricSourceAirflow:
		return "airflow"
	case MetricSourceAmazonEks:
		return "amazon_eks"
	case MetricSourceAmazonEksBlueprints:
		return "amazon_eks_blueprints"
	case MetricSourceAmazonMsk:
		return "amazon_msk"
	case MetricSourceAmbari:
		return "ambari"
	case MetricSourceApache:
		return "apache"
	case MetricSourceArangodb:
		return "arangodb"
	case MetricSourceArgoRollouts:
		return "argo_rollouts"
	case MetricSourceArgoWorkflows:
		return "argo_workflows"
	case MetricSourceArgocd:
		return "argocd"
	case MetricSourceAspdotnet:
		return "aspdotnet"
	case MetricSourceAviVantage:
		return "avi_vantage"
	case MetricSourceAzureActiveDirectory:
		return "azure_active_directory"
	case MetricSourceAzureIotEdge:
		return "azure_iot_edge"
	case MetricSourceBoundary:
		return "boundary"
	case MetricSourceBtrfs:
		return "btrfs"
	case MetricSourceCacti:
		return "cacti"
	case MetricSourceCalico:
		return "calico"
	case MetricSourceCassandraNodetool:
		return "cassandra_nodetool"
	case MetricSourceCeph:
		return "ceph"
	case MetricSourceCertManager:
		return "cert_manager"
	case MetricSourceCheckpointQuantumFirewall:
		return "checkpoint_quantum_firewall"
	case MetricSourceCilium:
		return "cilium"
	case MetricSourceCiscoAci:
		return "cisco_aci"
	case MetricSourceCiscoDuo:
		return "cisco_duo"
	case MetricSourceCiscoSecureFirewall:
		return "cisco_secure_firewall"
	case MetricSourceCiscoUmbrellaDNS:
		return "cisco_umbrella_dns"
	case MetricSourceCitrixHypervisor:
		return "citrix_hypervisor"
	case MetricSourceClickhouse:
		return "clickhouse"
	case MetricSourceCloudFoundryAPI:
		return "cloud_foundry_api"
	case MetricSourceCloudera:
		return "cloudera"
	case MetricSourceCockroachdb:
		return "cockroachdb"
	case MetricSourceConsul:
		return "consul"
	case MetricSourceConsulConnect:
		return "consul_connect"
	case MetricSourceCoredns:
		return "coredns"
	case MetricSourceCouch:
		return "couch"
	case MetricSourceCouchbase:
		return "couchbase"
	case MetricSourceCrio:
		return "crio"
	case MetricSourceDatabricks:
		return "databricks"
	case MetricSourceDatadogChecksBase:
		return "datadog_checks_base"
	case MetricSourceDatadogChecksDependencyProvider:
		return "datadog_checks_dependency_provider"
	case MetricSourceDatadogChecksDev:
		return "datadog_checks_dev"
	case MetricSourceDatadogChecksDownloader:
		return "datadog_checks_downloader"
	case MetricSourceDatadogChecksTestsHelper:
		return "datadog_checks_tests_helper"
	case MetricSourceDatadogClusterAgent:
		return "datadog_cluster_agent"
	case MetricSourceDatadogOperator:
		return "datadog_operator"
	case MetricSourceDcgm:
		return "dcgm"
	case MetricSourceDdev:
		return "ddev"
	case MetricSourceDirectory:
		return "directory"
	case MetricSourceDNSCheck:
		return "dns_check"
	case MetricSourceDockerDaemon:
		return "docker_daemon"
	case MetricSourceDotnetclr:
		return "dotnetclr"
	case MetricSourceDruid:
		return "druid"
	case MetricSourceEcsFargate:
		return "ecs_fargate"
	case MetricSourceEksAnywhere:
		return "eks_anywhere"
	case MetricSourceEksFargate:
		return "eks_fargate"
	case MetricSourceElastic:
		return "elastic"
	case MetricSourceEnvoy:
		return "envoy"
	case MetricSourceEtcd:
		return "etcd"
	case MetricSourceExchangeServer:
		return "exchange_server"
	case MetricSourceExternalDNS:
		return "external_dns"
	case MetricSourceFlink:
		return "flink"
	case MetricSourceFluentd:
		return "fluentd"
	case MetricSourceFluxcd:
		return "fluxcd"
	case MetricSourceFoundationdb:
		return "foundationdb"
	case MetricSourceGearmand:
		return "gearmand"
	case MetricSourceGitlab:
		return "gitlab"
	case MetricSourceGitlabRunner:
		return "gitlab_runner"
	case MetricSourceGke:
		return "gke"
	case MetricSourceGlusterfs:
		return "glusterfs"
	case MetricSourceGoMetro:
		return "go-metro"
	case MetricSourceGoExpvar:
		return "go_expvar"
	case MetricSourceGunicorn:
		return "gunicorn"
	case MetricSourceHaproxy:
		return "haproxy"
	case MetricSourceHarbor:
		return "harbor"
	case MetricSourceHdfsDatanode:
		return "hdfs_datanode"
	case MetricSourceHdfsNamenode:
		return "hdfs_namenode"
	case MetricSourceHTTPCheck:
		return "http_check"
	case MetricSourceHyperv:
		return "hyperv"
	case MetricSourceIamAccessAnalyzer:
		return "iam_access_analyzer"
	case MetricSourceIbmAce:
		return "ibm_ace"
	case MetricSourceIbmDb2:
		return "ibm_db2"
	case MetricSourceIbmI:
		return "ibm_i"
	case MetricSourceIbmMq:
		return "ibm_mq"
	case MetricSourceIbmWas:
		return "ibm_was"
	case MetricSourceIis:
		return "iis"
	case MetricSourceImpala:
		return "impala"
	case MetricSourceIstio:
		return "istio"
	case MetricSourceJmeter:
		return "jmeter"
	case MetricSourceJournald:
		return "journald"
	case MetricSourceKafkaConsumer:
		return "kafka_consumer"
	case MetricSourceKarpenter:
		return "karpenter"
	case MetricSourceKong:
		return "kong"
	case MetricSourceKubeAPIserverMetrics:
		return "kube_apiserver_metrics"
	case MetricSourceKubeControllerManager:
		return "kube_controller_manager"
	case MetricSourceKubeDNS:
		return "kube_dns"
	case MetricSourceKubeMetricsServer:
		return "kube_metrics_server"
	case MetricSourceKubeProxy:
		return "kube_proxy"
	case MetricSourceKubeScheduler:
		return "kube_scheduler"
	case MetricSourceKubelet:
		return "kubelet"
	case MetricSourceKubernetes:
		return "kubernetes"
	case MetricSourceKubernetesState:
		return "kubernetes_state"
	case MetricSourceKyototycoon:
		return "kyototycoon"
	case MetricSourceLangchain:
		return "langchain"
	case MetricSourceLighttpd:
		return "lighttpd"
	case MetricSourceLinkerd:
		return "linkerd"
	case MetricSourceLinuxProcExtras:
		return "linux_proc_extras"
	case MetricSourceMapr:
		return "mapr"
	case MetricSourceMapreduce:
		return "mapreduce"
	case MetricSourceMarathon:
		return "marathon"
	case MetricSourceMarklogic:
		return "marklogic"
	case MetricSourceMcache:
		return "mcache"
	case MetricSourceMesosMaster:
		return "mesos_master"
	case MetricSourceMesosSlave:
		return "mesos_slave"
	case MetricSourceMongo:
		return "mongo"
	case MetricSourceMysql:
		return "mysql"
	case MetricSourceNagios:
		return "nagios"
	case MetricSourceNfsstat:
		return "nfsstat"
	case MetricSourceNginx:
		return "nginx"
	case MetricSourceNginxIngressController:
		return "nginx_ingress_controller"
	case MetricSourceNvidiaJetson:
		return "nvidia_jetson"
	case MetricSourceNvidiaTriton:
		return "nvidia_triton"
	case MetricSourceOke:
		return "oke"
	case MetricSourceOpenai:
		return "openai"
	case MetricSourceOpenldap:
		return "openldap"
	case MetricSourceOpenmetrics:
		return "openmetrics"
	case MetricSourceOpenshift:
		return "openshift"
	case MetricSourceOpenstack:
		return "openstack"
	case MetricSourceOpenstackController:
		return "openstack_controller"
	case MetricSourceOracle:
		return "oracle"
	case MetricSourceOtel:
		return "otel"
	case MetricSourcePaloAltoPanorama:
		return "palo_alto_panorama"
	case MetricSourcePanFirewall:
		return "pan_firewall"
	case MetricSourcePdhCheck:
		return "pdh_check"
	case MetricSourcePgbouncer:
		return "pgbouncer"
	case MetricSourcePhpFpm:
		return "php_fpm"
	case MetricSourcePivotalPks:
		return "pivotal_pks"
	case MetricSourcePodman:
		return "podman"
	case MetricSourcePostfix:
		return "postfix"
	case MetricSourcePostgres:
		return "postgres"
	case MetricSourcePowerdnsRecursor:
		return "powerdns_recursor"
	case MetricSourceProcess:
		return "process"
	case MetricSourcePrometheus:
		return "prometheus"
	case MetricSourceProxysql:
		return "proxysql"
	case MetricSourcePulsar:
		return "pulsar"
	case MetricSourceRabbitmq:
		return "rabbitmq"
	case MetricSourceRay:
		return "ray"
	case MetricSourceRedisdb:
		return "redisdb"
	case MetricSourceRethinkdb:
		return "rethinkdb"
	case MetricSourceRiak:
		return "riak"
	case MetricSourceRiakcs:
		return "riakcs"
	case MetricSourceSapHana:
		return "sap_hana"
	case MetricSourceScylla:
		return "scylla"
	case MetricSourceSidekiq:
		return "sidekiq"
	case MetricSourceSilk:
		return "silk"
	case MetricSourceSinglestore:
		return "singlestore"
	case MetricSourceSnmpAmericanPowerConversion:
		return "snmp_american_power_conversion"
	case MetricSourceSnmpArista:
		return "snmp_arista"
	case MetricSourceSnmpAruba:
		return "snmp_aruba"
	case MetricSourceSnmpChatsworthProducts:
		return "snmp_chatsworth_products"
	case MetricSourceSnmpCheckPoint:
		return "snmp_check_point"
	case MetricSourceSnmpCisco:
		return "snmp_cisco"
	case MetricSourceSnmpDell:
		return "snmp_dell"
	case MetricSourceSnmpF5:
		return "snmp_f5"
	case MetricSourceSnmpFortinet:
		return "snmp_fortinet"
	case MetricSourceSnmpHewlettPackardEnterprise:
		return "snmp_hewlett_packard_enterprise"
	case MetricSourceSnmpJuniper:
		return "snmp_juniper"
	case MetricSourceSnmpNetapp:
		return "snmp_netapp"
	case MetricSourceSnowflake:
		return "snowflake"
	case MetricSourceSpark:
		return "spark"
	case MetricSourceSqlserver:
		return "sqlserver"
	case MetricSourceSquid:
		return "squid"
	case MetricSourceSSHCheck:
		return "ssh_check"
	case MetricSourceStatsd:
		return "statsd"
	case MetricSourceStrimzi:
		return "strimzi"
	case MetricSourceSupervisord:
		return "supervisord"
	case MetricSourceSystemCore:
		return "system_core"
	case MetricSourceSystemSwap:
		return "system_swap"
	case MetricSourceTCPCheck:
		return "tcp_check"
	case MetricSourceTeamcity:
		return "teamcity"
	case MetricSourceTekton:
		return "tekton"
	case MetricSourceTemporal:
		return "temporal"
	case MetricSourceTenable:
		return "tenable"
	case MetricSourceTeradata:
		return "teradata"
	case MetricSourceTerraform:
		return "terraform"
	case MetricSourceTLS:
		return "tls"
	case MetricSourceTokumx:
		return "tokumx"
	case MetricSourceTorchserve:
		return "torchserve"
	case MetricSourceTrafficServer:
		return "traffic_server"
	case MetricSourceTwemproxy:
		return "twemproxy"
	case MetricSourceTwistlock:
		return "twistlock"
	case MetricSourceVarnish:
		return "varnish"
	case MetricSourceVault:
		return "vault"
	case MetricSourceVertica:
		return "vertica"
	case MetricSourceVoltdb:
		return "voltdb"
	case MetricSourceVsphere:
		return "vsphere"
	case MetricSourceWeaviate:
		return "weaviate"
	case MetricSourceWin32EventLog:
		return "win32_event_log"
	case MetricSourceWincrashdetect:
		return "wincrashdetect"
	case MetricSourceWindowsPerformanceCounters:
		return "windows_performance_counters"
	case MetricSourceWindowsRegistry:
		return "windows_registry"
	case MetricSourceWindowsService:
		return "windows_service"
	case MetricSourceWmiCheck:
		return "wmi_check"
	case MetricSourceYarn:
		return "yarn"
	case MetricSourceZeek:
		return "zeek"
	case MetricSourceZk:
		return "zk"
	default:
		return "<unknown>"
	}
}

// CheckNameToMetricSource returns a MetricSource given the name
func CheckNameToMetricSource(name string) MetricSource {
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
	case "telemetry":
		return MetricSourceInternal
	case "active_directory":
		return MetricSourceActiveDirectory
	case "activemq_xml":
		return MetricSourceActivemqXML
	case "aerospike":
		return MetricSourceAerospike
	case "agent_metrics":
		return MetricSourceAgentMetrics
	case "airbyte":
		return MetricSourceAirbyte
	case "airflow":
		return MetricSourceAirflow
	case "amazon_eks":
		return MetricSourceAmazonEks
	case "amazon_eks_blueprints":
		return MetricSourceAmazonEksBlueprints
	case "amazon_msk":
		return MetricSourceAmazonMsk
	case "ambari":
		return MetricSourceAmbari
	case "apache":
		return MetricSourceApache
	case "arangodb":
		return MetricSourceArangodb
	case "argo_rollouts":
		return MetricSourceArgoRollouts
	case "argo_workflows":
		return MetricSourceArgoWorkflows
	case "argocd":
		return MetricSourceArgocd
	case "aspdotnet":
		return MetricSourceAspdotnet
	case "avi_vantage":
		return MetricSourceAviVantage
	case "azure_active_directory":
		return MetricSourceAzureActiveDirectory
	case "azure_iot_edge":
		return MetricSourceAzureIotEdge
	case "boundary":
		return MetricSourceBoundary
	case "btrfs":
		return MetricSourceBtrfs
	case "cacti":
		return MetricSourceCacti
	case "calico":
		return MetricSourceCalico
	case "cassandra_nodetool":
		return MetricSourceCassandraNodetool
	case "ceph":
		return MetricSourceCeph
	case "cert_manager":
		return MetricSourceCertManager
	case "checkpoint_quantum_firewall":
		return MetricSourceCheckpointQuantumFirewall
	case "cilium":
		return MetricSourceCilium
	case "cisco_aci":
		return MetricSourceCiscoAci
	case "cisco_duo":
		return MetricSourceCiscoDuo
	case "cisco_secure_firewall":
		return MetricSourceCiscoSecureFirewall
	case "cisco_umbrella_dns":
		return MetricSourceCiscoUmbrellaDNS
	case "citrix_hypervisor":
		return MetricSourceCitrixHypervisor
	case "clickhouse":
		return MetricSourceClickhouse
	case "cloud_foundry_api":
		return MetricSourceCloudFoundryAPI
	case "cloudera":
		return MetricSourceCloudera
	case "cockroachdb":
		return MetricSourceCockroachdb
	case "consul":
		return MetricSourceConsul
	case "consul_connect":
		return MetricSourceConsulConnect
	case "coredns":
		return MetricSourceCoredns
	case "couch":
		return MetricSourceCouch
	case "couchbase":
		return MetricSourceCouchbase
	case "crio":
		return MetricSourceCrio
	case "databricks":
		return MetricSourceDatabricks
	case "datadog_checks_base":
		return MetricSourceDatadogChecksBase
	case "datadog_checks_dependency_provider":
		return MetricSourceDatadogChecksDependencyProvider
	case "datadog_checks_dev":
		return MetricSourceDatadogChecksDev
	case "datadog_checks_downloader":
		return MetricSourceDatadogChecksDownloader
	case "datadog_checks_tests_helper":
		return MetricSourceDatadogChecksTestsHelper
	case "datadog_cluster_agent":
		return MetricSourceDatadogClusterAgent
	case "datadog_operator":
		return MetricSourceDatadogOperator
	case "dcgm":
		return MetricSourceDcgm
	case "ddev":
		return MetricSourceDdev
	case "directory":
		return MetricSourceDirectory
	case "dns_check":
		return MetricSourceDNSCheck
	case "docker_daemon":
		return MetricSourceDockerDaemon
	case "dotnetclr":
		return MetricSourceDotnetclr
	case "druid":
		return MetricSourceDruid
	case "ecs_fargate":
		return MetricSourceEcsFargate
	case "eks_anywhere":
		return MetricSourceEksAnywhere
	case "eks_fargate":
		return MetricSourceEksFargate
	case "elastic":
		return MetricSourceElastic
	case "envoy":
		return MetricSourceEnvoy
	case "etcd":
		return MetricSourceEtcd
	case "exchange_server":
		return MetricSourceExchangeServer
	case "external_dns":
		return MetricSourceExternalDNS
	case "flink":
		return MetricSourceFlink
	case "fluentd":
		return MetricSourceFluentd
	case "fluxcd":
		return MetricSourceFluxcd
	case "foundationdb":
		return MetricSourceFoundationdb
	case "gearmand":
		return MetricSourceGearmand
	case "gitlab":
		return MetricSourceGitlab
	case "gitlab_runner":
		return MetricSourceGitlabRunner
	case "gke":
		return MetricSourceGke
	case "glusterfs":
		return MetricSourceGlusterfs
	case "go-metro":
		return MetricSourceGoMetro
	case "go_expvar":
		return MetricSourceGoExpvar
	case "gunicorn":
		return MetricSourceGunicorn
	case "haproxy":
		return MetricSourceHaproxy
	case "harbor":
		return MetricSourceHarbor
	case "hdfs_datanode":
		return MetricSourceHdfsDatanode
	case "hdfs_namenode":
		return MetricSourceHdfsNamenode
	case "http_check":
		return MetricSourceHTTPCheck
	case "hyperv":
		return MetricSourceHyperv
	case "iam_access_analyzer":
		return MetricSourceIamAccessAnalyzer
	case "ibm_ace":
		return MetricSourceIbmAce
	case "ibm_db2":
		return MetricSourceIbmDb2
	case "ibm_i":
		return MetricSourceIbmI
	case "ibm_mq":
		return MetricSourceIbmMq
	case "ibm_was":
		return MetricSourceIbmWas
	case "iis":
		return MetricSourceIis
	case "impala":
		return MetricSourceImpala
	case "istio":
		return MetricSourceIstio
	case "jmeter":
		return MetricSourceJmeter
	case "journald":
		return MetricSourceJournald
	case "kafka_consumer":
		return MetricSourceKafkaConsumer
	case "karpenter":
		return MetricSourceKarpenter
	case "kong":
		return MetricSourceKong
	case "kube_apiserver_metrics":
		return MetricSourceKubeAPIserverMetrics
	case "kube_controller_manager":
		return MetricSourceKubeControllerManager
	case "kube_dns":
		return MetricSourceKubeDNS
	case "kube_metrics_server":
		return MetricSourceKubeMetricsServer
	case "kube_proxy":
		return MetricSourceKubeProxy
	case "kube_scheduler":
		return MetricSourceKubeScheduler
	case "kubelet":
		return MetricSourceKubelet
	case "kubernetes":
		return MetricSourceKubernetes
	case "kubernetes_state":
		return MetricSourceKubernetesState
	case "kyototycoon":
		return MetricSourceKyototycoon
	case "langchain":
		return MetricSourceLangchain
	case "lighttpd":
		return MetricSourceLighttpd
	case "linkerd":
		return MetricSourceLinkerd
	case "linux_proc_extras":
		return MetricSourceLinuxProcExtras
	case "mapr":
		return MetricSourceMapr
	case "mapreduce":
		return MetricSourceMapreduce
	case "marathon":
		return MetricSourceMarathon
	case "marklogic":
		return MetricSourceMarklogic
	case "mcache":
		return MetricSourceMcache
	case "mesos_master":
		return MetricSourceMesosMaster
	case "mesos_slave":
		return MetricSourceMesosSlave
	case "mongo":
		return MetricSourceMongo
	case "mysql":
		return MetricSourceMysql
	case "nagios":
		return MetricSourceNagios
	case "nfsstat":
		return MetricSourceNfsstat
	case "nginx":
		return MetricSourceNginx
	case "nginx_ingress_controller":
		return MetricSourceNginxIngressController
	case "nvidia_jetson":
		return MetricSourceNvidiaJetson
	case "nvidia_triton":
		return MetricSourceNvidiaTriton
	case "oke":
		return MetricSourceOke
	case "openai":
		return MetricSourceOpenai
	case "openldap":
		return MetricSourceOpenldap
	case "openmetrics":
		return MetricSourceOpenmetrics
	case "openshift":
		return MetricSourceOpenshift
	case "openstack":
		return MetricSourceOpenstack
	case "openstack_controller":
		return MetricSourceOpenstackController
	case "oracle":
		return MetricSourceOracle
	case "otel":
		return MetricSourceOtel
	case "palo_alto_panorama":
		return MetricSourcePaloAltoPanorama
	case "pan_firewall":
		return MetricSourcePanFirewall
	case "pdh_check":
		return MetricSourcePdhCheck
	case "pgbouncer":
		return MetricSourcePgbouncer
	case "php_fpm":
		return MetricSourcePhpFpm
	case "pivotal_pks":
		return MetricSourcePivotalPks
	case "podman":
		return MetricSourcePodman
	case "postfix":
		return MetricSourcePostfix
	case "postgres":
		return MetricSourcePostgres
	case "powerdns_recursor":
		return MetricSourcePowerdnsRecursor
	case "process":
		return MetricSourceProcess
	case "prometheus":
		return MetricSourcePrometheus
	case "proxysql":
		return MetricSourceProxysql
	case "pulsar":
		return MetricSourcePulsar
	case "rabbitmq":
		return MetricSourceRabbitmq
	case "ray":
		return MetricSourceRay
	case "redisdb":
		return MetricSourceRedisdb
	case "rethinkdb":
		return MetricSourceRethinkdb
	case "riak":
		return MetricSourceRiak
	case "riakcs":
		return MetricSourceRiakcs
	case "sap_hana":
		return MetricSourceSapHana
	case "scylla":
		return MetricSourceScylla
	case "sidekiq":
		return MetricSourceSidekiq
	case "silk":
		return MetricSourceSilk
	case "singlestore":
		return MetricSourceSinglestore
	case "snmp_american_power_conversion":
		return MetricSourceSnmpAmericanPowerConversion
	case "snmp_arista":
		return MetricSourceSnmpArista
	case "snmp_aruba":
		return MetricSourceSnmpAruba
	case "snmp_chatsworth_products":
		return MetricSourceSnmpChatsworthProducts
	case "snmp_check_point":
		return MetricSourceSnmpCheckPoint
	case "snmp_cisco":
		return MetricSourceSnmpCisco
	case "snmp_dell":
		return MetricSourceSnmpDell
	case "snmp_f5":
		return MetricSourceSnmpF5
	case "snmp_fortinet":
		return MetricSourceSnmpFortinet
	case "snmp_hewlett_packard_enterprise":
		return MetricSourceSnmpHewlettPackardEnterprise
	case "snmp_juniper":
		return MetricSourceSnmpJuniper
	case "snmp_netapp":
		return MetricSourceSnmpNetapp
	case "snowflake":
		return MetricSourceSnowflake
	case "spark":
		return MetricSourceSpark
	case "sqlserver":
		return MetricSourceSqlserver
	case "squid":
		return MetricSourceSquid
	case "ssh_check":
		return MetricSourceSSHCheck
	case "statsd":
		return MetricSourceStatsd
	case "strimzi":
		return MetricSourceStrimzi
	case "supervisord":
		return MetricSourceSupervisord
	case "system_core":
		return MetricSourceSystemCore
	case "system_swap":
		return MetricSourceSystemSwap
	case "tcp_check":
		return MetricSourceTCPCheck
	case "teamcity":
		return MetricSourceTeamcity
	case "tekton":
		return MetricSourceTekton
	case "temporal":
		return MetricSourceTemporal
	case "tenable":
		return MetricSourceTenable
	case "teradata":
		return MetricSourceTeradata
	case "terraform":
		return MetricSourceTerraform
	case "tls":
		return MetricSourceTLS
	case "tokumx":
		return MetricSourceTokumx
	case "torchserve":
		return MetricSourceTorchserve
	case "traffic_server":
		return MetricSourceTrafficServer
	case "twemproxy":
		return MetricSourceTwemproxy
	case "twistlock":
		return MetricSourceTwistlock
	case "varnish":
		return MetricSourceVarnish
	case "vault":
		return MetricSourceVault
	case "vertica":
		return MetricSourceVertica
	case "voltdb":
		return MetricSourceVoltdb
	case "vsphere":
		return MetricSourceVsphere
	case "weaviate":
		return MetricSourceWeaviate
	case "win32_event_log":
		return MetricSourceWin32EventLog
	case "wincrashdetect":
		return MetricSourceWincrashdetect
	case "windows_performance_counters":
		return MetricSourceWindowsPerformanceCounters
	case "windows_registry":
		return MetricSourceWindowsRegistry
	case "windows_service":
		return MetricSourceWindowsService
	case "wmi_check":
		return MetricSourceWmiCheck
	case "yarn":
		return MetricSourceYarn
	case "zeek":
		return MetricSourceZeek
	case "zk":
		return MetricSourceZk
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
