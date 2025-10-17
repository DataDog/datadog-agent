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
	MetricSourceNTP
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
	MetricSourceCloudFoundry
	MetricSourceJenkins
	MetricSourceGPU
	MetricSourceWlan
	MetricSourceWindowsCertificateStore

	// Python Checks
	MetricSourceZenohRouter
	MetricSourceZabbix
	MetricSourceWayfinder
	MetricSourceVespa
	MetricSourceUpsc
	MetricSourceUpboundUxp
	MetricSourceUnifiConsole
	MetricSourceUnbound
	MetricSourceTraefik
	MetricSourceTidb
	MetricSourceSyncthing
	MetricSourceStorm
	MetricSourceStardog
	MetricSourceSpeedtest
	MetricSourceSortdb
	MetricSourceSonarr
	MetricSourceSnmpwalk
	MetricSourceSendmail
	MetricSourceScaphandre
	MetricSourceScalr
	MetricSourceRiakRepl
	MetricSourceRedpanda
	MetricSourceRedisenterprise
	MetricSourceRedisSentinel
	MetricSourceRebootRequired
	MetricSourceRadarr
	MetricSourcePurefb
	MetricSourcePurefa
	MetricSourcePuma
	MetricSourcePortworx
	MetricSourcePing
	MetricSourcePihole
	MetricSourcePhpOpcache
	MetricSourcePhpApcu
	MetricSourceOpenPolicyAgent
	MetricSourceOctopusDeploy
	MetricSourceOctoprint
	MetricSourceNvml
	MetricSourceNs1
	MetricSourceNnSdwan
	MetricSourceNextcloud
	MetricSourceNeutrona
	MetricSourceNeo4j
	MetricSourceMergify
	MetricSourceLogstash
	MetricSourceLighthouse
	MetricSourceKernelcare
	MetricSourceKepler
	MetricSourceJfrogPlatformSelfHosted
	MetricSourceHikaricp
	MetricSourceGrpcCheck
	MetricSourceGoPprofScraper
	MetricSourceGnatsdStreaming
	MetricSourceGnatsd
	MetricSourceGitea
	MetricSourceGatekeeper
	MetricSourceFlyIo
	MetricSourceFluentbit
	MetricSourceFilemage
	MetricSourceFilebeat
	MetricSourceFiddler
	MetricSourceExim
	MetricSourceEventstore
	MetricSourceEmqx
	MetricSourceCyral
	MetricSourceCybersixgillActionableAlerts
	MetricSourceCloudsmith
	MetricSourceCloudnatix
	MetricSourceCfssl
	MetricSourceBind9
	MetricSourceAwsPricing
	MetricSourceAqua
	MetricSourceKubernetesClusterAutoscaler
	MetricSourceKubeVirtAPI
	MetricSourceKubeVirtController
	MetricSourceKubeVirtHandler
	MetricSourceTraefikMesh
	MetricSourceWeaviate
	MetricSourceTorchserve
	MetricSourceTemporal
	MetricSourceTeleport
	MetricSourceTekton
	MetricSourceStrimzi
	MetricSourceRay
	MetricSourceNvidiaTriton
	MetricSourceKarpenter
	MetricSourceFluxcd
	MetricSourceEsxi
	MetricSourceDcgm
	MetricSourceDatadogClusterAgent
	MetricSourceCloudera
	MetricSourceArgoWorkflows
	MetricSourceArgoRollouts
	MetricSourceActiveDirectory
	MetricSourceActivemqXML
	MetricSourceAerospike
	MetricSourceAirflow
	MetricSourceAmazonMsk
	MetricSourceAmbari
	MetricSourceApache
	MetricSourceArangodb
	MetricSourceArgocd
	MetricSourceAspdotnet
	MetricSourceAviVantage
	MetricSourceAzureIotEdge
	MetricSourceBoundary
	MetricSourceBtrfs
	MetricSourceCacti
	MetricSourceCalico
	MetricSourceCassandraNodetool
	MetricSourceCeph
	MetricSourceCertManager
	MetricSourceCilium
	MetricSourceCitrixHypervisor
	MetricSourceClickhouse
	MetricSourceCloudFoundryAPI
	MetricSourceCockroachdb
	MetricSourceConsul
	MetricSourceCoredns
	MetricSourceCouch
	MetricSourceCouchbase
	MetricSourceCrio
	MetricSourceDirectory
	MetricSourceDNSCheck
	MetricSourceDotnetclr
	MetricSourceDruid
	MetricSourceEcsFargate
	MetricSourceEksFargate
	MetricSourceElastic
	MetricSourceEnvoy
	MetricSourceEtcd
	MetricSourceExchangeServer
	MetricSourceExternalDNS
	MetricSourceFluentd
	MetricSourceFoundationdb
	MetricSourceGearmand
	MetricSourceGitlab
	MetricSourceGitlabRunner
	MetricSourceGlusterfs
	MetricSourceGoExpvar
	MetricSourceGunicorn
	MetricSourceHaproxy
	MetricSourceHarbor
	MetricSourceHdfsDatanode
	MetricSourceHdfsNamenode
	MetricSourceHTTPCheck
	MetricSourceHyperv
	MetricSourceIbmAce
	MetricSourceIbmDb2
	MetricSourceIbmI
	MetricSourceIbmMq
	MetricSourceIbmWas
	MetricSourceIis
	MetricSourceImpala
	MetricSourceIstio
	MetricSourceKafkaConsumer
	MetricSourceKong
	MetricSourceKubeAPIserverMetrics
	MetricSourceKubeControllerManager
	MetricSourceKubeDNS
	MetricSourceKubeMetricsServer
	MetricSourceKubeProxy
	MetricSourceKubeScheduler
	MetricSourceKubelet
	MetricSourceKubernetesState
	MetricSourceKyototycoon
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
	MetricSourceOpenldap
	MetricSourceOpenmetrics
	MetricSourceOpenstack
	MetricSourceOpenstackController
	MetricSourceOracle
	MetricSourcePdhCheck
	MetricSourcePgbouncer
	MetricSourcePhpFpm
	MetricSourcePostfix
	MetricSourcePostgres
	MetricSourcePowerdnsRecursor
	MetricSourceProcess
	MetricSourcePrometheus
	MetricSourceProxysql
	MetricSourcePulsar
	MetricSourceRabbitmq
	MetricSourceRedisdb
	MetricSourceRethinkdb
	MetricSourceRiak
	MetricSourceRiakcs
	MetricSourceSapHana
	MetricSourceScylla
	MetricSourceSilk
	MetricSourceSinglestore
	MetricSourceSnowflake
	MetricSourceSpark
	MetricSourceSqlserver
	MetricSourceSquid
	MetricSourceSSHCheck
	MetricSourceStatsd
	MetricSourceSupervisord
	MetricSourceSystemCore
	MetricSourceSystemSwap
	MetricSourceTCPCheck
	MetricSourceTeamcity
	MetricSourceTeradata
	MetricSourceTLS
	MetricSourceTokumx
	MetricSourceTrafficServer
	MetricSourceTwemproxy
	MetricSourceTwistlock
	MetricSourceVarnish
	MetricSourceVault
	MetricSourceVertica
	MetricSourceVllm
	MetricSourceVoltdb
	MetricSourceVsphere
	MetricSourceWin32EventLog
	MetricSourceWindowsPerformanceCounters
	MetricSourceWindowsService
	MetricSourceWmiCheck
	MetricSourceYarn
	MetricSourceZk
	MetricSourceAwsNeuron
	MetricSourceTibcoEMS
	MetricSourceSlurm
	MetricSourceKyverno
	MetricSourceKubeflow
	MetricSourceAppgateSDP
	MetricSourceAnyscale
	MetricSourceMilvus
	MetricSourceNvidiaNim
	MetricSourceQuarkus
	MetricSourceVelero
	MetricSourceCelery
	MetricSourceInfiniband
	MetricSourceSilverstripeCMS
	MetricSourceAnecdote
	MetricSourceSonatypeNexus
	MetricSourceAltairPBSPro
	MetricSourceFalco
	MetricSourceKrakenD
	MetricSourceKuma
	MetricSourceLiteLLM
	MetricSourceLustre
	MetricSourceProxmox
	MetricSourceResilience4j
	MetricSourceSupabase
	MetricSourceKeda
	MetricSourceDuckdb
	MetricSourceBentoMl
	MetricSourceHuggingFaceTgi
	MetricSourceIbmSpectrumLsf
	MetricSourceDatadogOperator

	// OpenTelemetry Collector receivers
	MetricSourceOpenTelemetryCollectorUnknown
	MetricSourceOpenTelemetryCollectorDockerstatsReceiver
	MetricSourceOpenTelemetryCollectorElasticsearchReceiver
	MetricSourceOpenTelemetryCollectorExpvarReceiver
	MetricSourceOpenTelemetryCollectorFilestatsReceiver
	MetricSourceOpenTelemetryCollectorFlinkmetricsReceiver
	MetricSourceOpenTelemetryCollectorGitproviderReceiver
	MetricSourceOpenTelemetryCollectorHaproxyReceiver
	MetricSourceOpenTelemetryCollectorHostmetricsReceiver
	MetricSourceOpenTelemetryCollectorHttpcheckReceiver
	MetricSourceOpenTelemetryCollectorIisReceiver
	MetricSourceOpenTelemetryCollectorK8sclusterReceiver
	MetricSourceOpenTelemetryCollectorKafkametricsReceiver
	MetricSourceOpenTelemetryCollectorKubeletstatsReceiver
	MetricSourceOpenTelemetryCollectorMemcachedReceiver
	MetricSourceOpenTelemetryCollectorMongodbatlasReceiver
	MetricSourceOpenTelemetryCollectorMongodbReceiver
	MetricSourceOpenTelemetryCollectorMysqlReceiver
	MetricSourceOpenTelemetryCollectorNginxReceiver
	MetricSourceOpenTelemetryCollectorNsxtReceiver
	MetricSourceOpenTelemetryCollectorOracledbReceiver
	MetricSourceOpenTelemetryCollectorPostgresqlReceiver
	MetricSourceOpenTelemetryCollectorPrometheusReceiver
	MetricSourceOpenTelemetryCollectorRabbitmqReceiver
	MetricSourceOpenTelemetryCollectorRedisReceiver
	MetricSourceOpenTelemetryCollectorRiakReceiver
	MetricSourceOpenTelemetryCollectorSaphanaReceiver
	MetricSourceOpenTelemetryCollectorSnmpReceiver
	MetricSourceOpenTelemetryCollectorSnowflakeReceiver
	MetricSourceOpenTelemetryCollectorSplunkenterpriseReceiver
	MetricSourceOpenTelemetryCollectorSqlserverReceiver
	MetricSourceOpenTelemetryCollectorSshcheckReceiver
	MetricSourceOpenTelemetryCollectorStatsdReceiver
	MetricSourceOpenTelemetryCollectorVcenterReceiver
	MetricSourceOpenTelemetryCollectorZookeeperReceiver
	MetricSourceOpenTelemetryCollectorActiveDirectorydsReceiver
	MetricSourceOpenTelemetryCollectorAerospikeReceiver
	MetricSourceOpenTelemetryCollectorApacheReceiver
	MetricSourceOpenTelemetryCollectorApachesparkReceiver
	MetricSourceOpenTelemetryCollectorAzuremonitorReceiver
	MetricSourceOpenTelemetryCollectorBigipReceiver
	MetricSourceOpenTelemetryCollectorChronyReceiver
	MetricSourceOpenTelemetryCollectorCouchdbReceiver

	// Serverless
	MetricSourceServerless
	MetricSourceAwsLambdaCustom
	MetricSourceAwsLambdaEnhanced
	MetricSourceAwsLambdaRuntime
	MetricSourceAzureContainerAppCustom
	MetricSourceAzureContainerAppEnhanced
	MetricSourceAzureContainerAppRuntime
	MetricSourceAzureAppServiceCustom
	MetricSourceAzureAppServiceEnhanced
	MetricSourceAzureAppServiceRuntime
	MetricSourceGoogleCloudRunCustom
	MetricSourceGoogleCloudRunEnhanced
	MetricSourceGoogleCloudRunRuntime
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
	case MetricSourceNTP:
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
	case MetricSourceAirflow:
		return "airflow"
	case MetricSourceAmazonMsk:
		return "amazon_msk"
	case MetricSourceAmbari:
		return "ambari"
	case MetricSourceApache:
		return "apache"
	case MetricSourceArangodb:
		return "arangodb"
	case MetricSourceArgocd:
		return "argocd"
	case MetricSourceAspdotnet:
		return "aspdotnet"
	case MetricSourceAviVantage:
		return "avi_vantage"
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
	case MetricSourceCilium:
		return "cilium"
	case MetricSourceCitrixHypervisor:
		return "citrix_hypervisor"
	case MetricSourceClickhouse:
		return "clickhouse"
	case MetricSourceCloudFoundry:
		return "cloudfoundry"
	case MetricSourceCloudFoundryAPI:
		return "cloud_foundry_api"
	case MetricSourceCockroachdb:
		return "cockroachdb"
	case MetricSourceConsul:
		return "consul"
	case MetricSourceCoredns:
		return "coredns"
	case MetricSourceCouch:
		return "couch"
	case MetricSourceCouchbase:
		return "couchbase"
	case MetricSourceCrio:
		return "crio"
	case MetricSourceDirectory:
		return "directory"
	case MetricSourceDNSCheck:
		return "dns_check"
	case MetricSourceDotnetclr:
		return "dotnetclr"
	case MetricSourceDruid:
		return "druid"
	case MetricSourceEcsFargate:
		return "ecs_fargate"
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
	case MetricSourceFluentd:
		return "fluentd"
	case MetricSourceFlyIo:
		return "fly_io"
	case MetricSourceFoundationdb:
		return "foundationdb"
	case MetricSourceGearmand:
		return "gearmand"
	case MetricSourceGitlab:
		return "gitlab"
	case MetricSourceGitlabRunner:
		return "gitlab_runner"
	case MetricSourceGlusterfs:
		return "glusterfs"
	case MetricSourceGoExpvar:
		return "go_expvar"
	case MetricSourceGPU:
		return "gpu"
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
	case MetricSourceJenkins:
		return "jenkins"
	case MetricSourceKafkaConsumer:
		return "kafka_consumer"
	case MetricSourceKepler:
		return "kepler"
	case MetricSourceKong:
		return "kong"
	case MetricSourceKubeAPIserverMetrics:
		return "kube_apiserver_metrics"
	case MetricSourceKubeControllerManager:
		return "kube_controller_manager"
	case MetricSourceKubeDNS:
		return "kube_dns"
	case MetricSourceKubeflow:
		return "kubeflow"
	case MetricSourceKubeMetricsServer:
		return "kube_metrics_server"
	case MetricSourceKubeProxy:
		return "kube_proxy"
	case MetricSourceKubeScheduler:
		return "kube_scheduler"
	case MetricSourceKubelet:
		return "kubelet"
	case MetricSourceKubernetesState:
		return "kubernetes_state"
	case MetricSourceKyototycoon:
		return "kyototycoon"
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
	case MetricSourceOpenldap:
		return "openldap"
	case MetricSourceOpenmetrics:
		return "openmetrics"
	case MetricSourceOpenstack:
		return "openstack"
	case MetricSourceOpenstackController:
		return "openstack_controller"
	case MetricSourceOracle:
		return "oracle"
	case MetricSourcePdhCheck:
		return "pdh_check"
	case MetricSourcePgbouncer:
		return "pgbouncer"
	case MetricSourcePhpFpm:
		return "php_fpm"
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
	case MetricSourceSilk:
		return "silk"
	case MetricSourceSinglestore:
		return "singlestore"
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
	case MetricSourceTeradata:
		return "teradata"
	case MetricSourceTLS:
		return "tls"
	case MetricSourceTokumx:
		return "tokumx"
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
	case MetricSourceVllm:
		return "vllm"
	case MetricSourceVoltdb:
		return "voltdb"
	case MetricSourceVsphere:
		return "vsphere"
	case MetricSourceWin32EventLog:
		return "win32_event_log"
	case MetricSourceWindowsPerformanceCounters:
		return "windows_performance_counters"
	case MetricSourceWindowsService:
		return "windows_service"
	case MetricSourceWmiCheck:
		return "wmi_check"
	case MetricSourceYarn:
		return "yarn"
	case MetricSourceZk:
		return "zk"
	case MetricSourceArgoRollouts:
		return "argo_rollouts"
	case MetricSourceArgoWorkflows:
		return "argo_workflows"
	case MetricSourceCloudera:
		return "cloudera"
	case MetricSourceDatadogClusterAgent:
		return "datadog_cluster_agent"
	case MetricSourceDcgm:
		return "dcgm"
	case MetricSourceEsxi:
		return "esxi"
	case MetricSourceFluxcd:
		return "fluxcd"
	case MetricSourceKarpenter:
		return "karpenter"
	case MetricSourceNvidiaTriton:
		return "nvidia_triton"
	case MetricSourceNvidiaNim:
		return "nvidia_nim"
	case MetricSourceRay:
		return "ray"
	case MetricSourceStrimzi:
		return "strimzi"
	case MetricSourceTekton:
		return "tekton"
	case MetricSourceTeleport:
		return "teleport"
	case MetricSourceTemporal:
		return "temporal"
	case MetricSourceTorchserve:
		return "torchserve"
	case MetricSourceWeaviate:
		return "weaviate"
	case MetricSourceTraefikMesh:
		return "traefik_mesh"
	case MetricSourceKubernetesClusterAutoscaler:
		return "kubernetes_cluster_autoscaler"
	case MetricSourceAqua:
		return "aqua"
	case MetricSourceAwsPricing:
		return "aws_pricing"
	case MetricSourceBind9:
		return "bind9"
	case MetricSourceCfssl:
		return "cfssl"
	case MetricSourceCloudnatix:
		return "cloudnatix"
	case MetricSourceCloudsmith:
		return "cloudsmith"
	case MetricSourceCybersixgillActionableAlerts:
		return "cybersixgill_actionable_alerts"
	case MetricSourceCyral:
		return "cyral"
	case MetricSourceEmqx:
		return "emqx"
	case MetricSourceEventstore:
		return "eventstore"
	case MetricSourceExim:
		return "exim"
	case MetricSourceFiddler:
		return "fiddler"
	case MetricSourceFilebeat:
		return "filebeat"
	case MetricSourceFilemage:
		return "filemage"
	case MetricSourceFluentbit:
		return "fluentbit"
	case MetricSourceGatekeeper:
		return "gatekeeper"
	case MetricSourceGitea:
		return "gitea"
	case MetricSourceGnatsd:
		return "gnatsd"
	case MetricSourceGnatsdStreaming:
		return "gnatsd_streaming"
	case MetricSourceGoPprofScraper:
		return "go_pprof_scraper"
	case MetricSourceGrpcCheck:
		return "grpc_check"
	case MetricSourceHikaricp:
		return "hikaricp"
	case MetricSourceJfrogPlatformSelfHosted:
		return "jfrog_platform_self_hosted"
	case MetricSourceKernelcare:
		return "kernelcare"
	case MetricSourceLighthouse:
		return "lighthouse"
	case MetricSourceLogstash:
		return "logstash"
	case MetricSourceMergify:
		return "mergify"
	case MetricSourceNeo4j:
		return "neo4j"
	case MetricSourceNeutrona:
		return "neutrona"
	case MetricSourceNextcloud:
		return "nextcloud"
	case MetricSourceNnSdwan:
		return "nn_sdwan"
	case MetricSourceNs1:
		return "ns1"
	case MetricSourceNvml:
		return "nvml"
	case MetricSourceOctoprint:
		return "octoprint"
	case MetricSourceOctopusDeploy:
		return "octopus_deploy"
	case MetricSourceOpenPolicyAgent:
		return "open_policy_agent"
	case MetricSourcePhpApcu:
		return "php_apcu"
	case MetricSourcePhpOpcache:
		return "php_opcache"
	case MetricSourcePihole:
		return "pihole"
	case MetricSourcePing:
		return "ping"
	case MetricSourcePortworx:
		return "portworx"
	case MetricSourcePuma:
		return "puma"
	case MetricSourcePurefa:
		return "purefa"
	case MetricSourcePurefb:
		return "purefb"
	case MetricSourceRadarr:
		return "radarr"
	case MetricSourceRebootRequired:
		return "reboot_required"
	case MetricSourceRedisSentinel:
		return "redis_sentinel"
	case MetricSourceRedisenterprise:
		return "redisenterprise"
	case MetricSourceRedpanda:
		return "redpanda"
	case MetricSourceRiakRepl:
		return "riak_repl"
	case MetricSourceScalr:
		return "scalr"
	case MetricSourceScaphandre:
		return "scaphandre"
	case MetricSourceSendmail:
		return "sendmail"
	case MetricSourceSnmpwalk:
		return "snmpwalk"
	case MetricSourceSonarr:
		return "sonarr"
	case MetricSourceSortdb:
		return "sortdb"
	case MetricSourceSpeedtest:
		return "speedtest"
	case MetricSourceStardog:
		return "stardog"
	case MetricSourceStorm:
		return "storm"
	case MetricSourceSyncthing:
		return "syncthing"
	case MetricSourceTidb:
		return "tidb"
	case MetricSourceTraefik:
		return "traefik"
	case MetricSourceUnbound:
		return "unbound"
	case MetricSourceUnifiConsole:
		return "unifi_console"
	case MetricSourceUpboundUxp:
		return "upbound_uxp"
	case MetricSourceUpsc:
		return "upsc"
	case MetricSourceVespa:
		return "vespa"
	case MetricSourceWayfinder:
		return "wayfinder"
	case MetricSourceZabbix:
		return "zabbix"
	case MetricSourceZenohRouter:
		return "zenoh_router"
	case MetricSourceAwsNeuron:
		return "aws_neuron"
	case MetricSourceMilvus:
		return "milvus"
	case MetricSourceQuarkus:
		return "quarkus"
	case MetricSourceVelero:
		return "velero"
	case MetricSourceCelery:
		return "celery"
	case MetricSourceInfiniband:
		return "infiniband"
	case MetricSourceAltairPBSPro:
		return "altair_pbs_pro"
	case MetricSourceFalco:
		return "falco"
	case MetricSourceKrakenD:
		return "krakend"
	case MetricSourceKuma:
		return "kuma"
	case MetricSourceLiteLLM:
		return "lite_llm"
	case MetricSourceLustre:
		return "lustre"
	case MetricSourceProxmox:
		return "proxmox"
	case MetricSourceResilience4j:
		return "resilience4j"
	case MetricSourceSupabase:
		return "supabase"
	case MetricSourceKeda:
		return "keda"
	case MetricSourceDuckdb:
		return "duckdb"
	case MetricSourceBentoMl:
		return "bentoml"
	case MetricSourceHuggingFaceTgi:
		return "hugging_face_tgi"
	case MetricSourceIbmSpectrumLsf:
		return "ibm_spectrum_lsf"
	case MetricSourceDatadogOperator:
		return "datadog_operator"
	case MetricSourceOpenTelemetryCollectorUnknown:
		return "opentelemetry_collector_unknown"
	case MetricSourceOpenTelemetryCollectorDockerstatsReceiver:
		return "opentelemetry_collector_dockerstatsreceiver"
	case MetricSourceOpenTelemetryCollectorElasticsearchReceiver:
		return "opentelemetry_collector_elasticsearchreceiver"
	case MetricSourceOpenTelemetryCollectorExpvarReceiver:
		return "opentelemetry_collector_expvarreceiver"
	case MetricSourceOpenTelemetryCollectorFilestatsReceiver:
		return "opentelemetry_collector_filestatsreceiver"
	case MetricSourceOpenTelemetryCollectorFlinkmetricsReceiver:
		return "opentelemetry_collector_flinkmetricsreceiver"
	case MetricSourceOpenTelemetryCollectorGitproviderReceiver:
		return "opentelemetry_collector_gitproviderreceiver"
	case MetricSourceOpenTelemetryCollectorHaproxyReceiver:
		return "opentelemetry_collector_haproxyreceiver"
	case MetricSourceOpenTelemetryCollectorHostmetricsReceiver:
		return "opentelemetry_collector_hostmetricsreceiver"
	case MetricSourceOpenTelemetryCollectorHttpcheckReceiver:
		return "opentelemetry_collector_httpcheckreceiver"
	case MetricSourceOpenTelemetryCollectorIisReceiver:
		return "opentelemetry_collector_iisreceiver"
	case MetricSourceOpenTelemetryCollectorK8sclusterReceiver:
		return "opentelemetry_collector_k8sclusterreceiver"
	case MetricSourceOpenTelemetryCollectorKafkametricsReceiver:
		return "opentelemetry_collector_kafkametricsreceiver"
	case MetricSourceOpenTelemetryCollectorKubeletstatsReceiver:
		return "opentelemetry_collector_kubeletstatsreceiver"
	case MetricSourceOpenTelemetryCollectorMemcachedReceiver:
		return "opentelemetry_collector_memcachedreceiver"
	case MetricSourceOpenTelemetryCollectorMongodbatlasReceiver:
		return "opentelemetry_collector_mongodbatlasreceiver"
	case MetricSourceOpenTelemetryCollectorMongodbReceiver:
		return "opentelemetry_collector_mongodbreceiver"
	case MetricSourceOpenTelemetryCollectorMysqlReceiver:
		return "opentelemetry_collector_mysqlreceiver"
	case MetricSourceOpenTelemetryCollectorNginxReceiver:
		return "opentelemetry_collector_nginxreceiver"
	case MetricSourceOpenTelemetryCollectorNsxtReceiver:
		return "opentelemetry_collector_nsxtreceiver"
	case MetricSourceOpenTelemetryCollectorOracledbReceiver:
		return "opentelemetry_collector_oracledbreceiver"
	case MetricSourceOpenTelemetryCollectorPostgresqlReceiver:
		return "opentelemetry_collector_postgresqlreceiver"
	case MetricSourceOpenTelemetryCollectorPrometheusReceiver:
		return "opentelemetry_collector_prometheusreceiver"
	case MetricSourceOpenTelemetryCollectorRabbitmqReceiver:
		return "opentelemetry_collector_rabbitmqreceiver"
	case MetricSourceOpenTelemetryCollectorRedisReceiver:
		return "opentelemetry_collector_redisreceiver"
	case MetricSourceOpenTelemetryCollectorRiakReceiver:
		return "opentelemetry_collector_riakreceiver"
	case MetricSourceOpenTelemetryCollectorSaphanaReceiver:
		return "opentelemetry_collector_saphanareceiver"
	case MetricSourceOpenTelemetryCollectorSnmpReceiver:
		return "opentelemetry_collector_snmpreceiver"
	case MetricSourceOpenTelemetryCollectorSnowflakeReceiver:
		return "opentelemetry_collector_snowflakereceiver"
	case MetricSourceOpenTelemetryCollectorSplunkenterpriseReceiver:
		return "opentelemetry_collector_splunkenterprisereceiver"
	case MetricSourceOpenTelemetryCollectorSqlserverReceiver:
		return "opentelemetry_collector_sqlserverreceiver"
	case MetricSourceOpenTelemetryCollectorSshcheckReceiver:
		return "opentelemetry_collector_sshcheckreceiver"
	case MetricSourceOpenTelemetryCollectorStatsdReceiver:
		return "opentelemetry_collector_statsdreceiver"
	case MetricSourceOpenTelemetryCollectorVcenterReceiver:
		return "opentelemetry_collector_vcenterreceiver"
	case MetricSourceOpenTelemetryCollectorZookeeperReceiver:
		return "opentelemetry_collector_zookeeperreceiver"
	case MetricSourceOpenTelemetryCollectorActiveDirectorydsReceiver:
		return "opentelemetry_collector_activedirectorydsreceiver"
	case MetricSourceOpenTelemetryCollectorAerospikeReceiver:
		return "opentelemetry_collector_aerospikereceiver"
	case MetricSourceOpenTelemetryCollectorApacheReceiver:
		return "opentelemetry_collector_apachereceiver"
	case MetricSourceOpenTelemetryCollectorApachesparkReceiver:
		return "opentelemetry_collector_apachesparkreceiver"
	case MetricSourceOpenTelemetryCollectorAzuremonitorReceiver:
		return "opentelemetry_collector_azuremonitorreceiver"
	case MetricSourceOpenTelemetryCollectorBigipReceiver:
		return "opentelemetry_collector_bigipreceiver"
	case MetricSourceOpenTelemetryCollectorChronyReceiver:
		return "opentelemetry_collector_chronyreceiver"
	case MetricSourceOpenTelemetryCollectorCouchdbReceiver:
		return "opentelemetry_collector_couchdbreceiver"
	case MetricSourceServerless:
		return "serverless"
	case MetricSourceAwsLambdaCustom:
		return "aws_lambda_custom"
	case MetricSourceAwsLambdaEnhanced:
		return "aws_lambda_enhanced"
	case MetricSourceAwsLambdaRuntime:
		return "aws_lambda_runtime"
	case MetricSourceAzureContainerAppCustom:
		return "azure_container_app_custom"
	case MetricSourceAzureContainerAppEnhanced:
		return "azure_container_app_enhanced"
	case MetricSourceAzureContainerAppRuntime:
		return "azure_container_app_runtime"
	case MetricSourceAzureAppServiceCustom:
		return "azure_app_service_custom"
	case MetricSourceAzureAppServiceEnhanced:
		return "azure_app_service_enhanced"
	case MetricSourceAzureAppServiceRuntime:
		return "azure_app_service_runtime"
	case MetricSourceGoogleCloudRunCustom:
		return "google_cloud_run_custom"
	case MetricSourceGoogleCloudRunEnhanced:
		return "google_cloud_run_enhanced"
	case MetricSourceGoogleCloudRunRuntime:
		return "google_cloud_run_runtime"
	case MetricSourceWlan:
		return "wlan"
	case MetricSourceWindowsCertificateStore:
		return "windows_certificate"
	default:
		return "<unknown>"
	}
}

// CheckNameToMetricSource returns a MetricSource given the name
func CheckNameToMetricSource(name string) MetricSource {
	switch name {
	case "anecdote":
		return MetricSourceAnecdote
	case "container":
		return MetricSourceContainer
	case "containerd":
		return MetricSourceContainerd
	case "cri":
		return MetricSourceCri
	case "docker":
		return MetricSourceDocker
	case "ntp":
		return MetricSourceNTP
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
	case "airflow":
		return MetricSourceAirflow
	case "amazon_msk":
		return MetricSourceAmazonMsk
	case "ambari":
		return MetricSourceAmbari
	case "apache":
		return MetricSourceApache
	case "arangodb":
		return MetricSourceArangodb
	case "argocd":
		return MetricSourceArgocd
	case "aspdotnet":
		return MetricSourceAspdotnet
	case "avi_vantage":
		return MetricSourceAviVantage
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
	case "cilium":
		return MetricSourceCilium
	case "citrix_hypervisor":
		return MetricSourceCitrixHypervisor
	case "clickhouse":
		return MetricSourceClickhouse
	case "cloud_foundry_api":
		return MetricSourceCloudFoundryAPI
	case "cockroachdb":
		return MetricSourceCockroachdb
	case "consul":
		return MetricSourceConsul
	case "coredns":
		return MetricSourceCoredns
	case "couch":
		return MetricSourceCouch
	case "couchbase":
		return MetricSourceCouchbase
	case "crio":
		return MetricSourceCrio
	case "directory":
		return MetricSourceDirectory
	case "dns_check":
		return MetricSourceDNSCheck
	case "dotnetclr":
		return MetricSourceDotnetclr
	case "druid":
		return MetricSourceDruid
	case "ecs_fargate":
		return MetricSourceEcsFargate
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
	case "fluentd":
		return MetricSourceFluentd
	case "foundationdb":
		return MetricSourceFoundationdb
	case "gearmand":
		return MetricSourceGearmand
	case "gitlab":
		return MetricSourceGitlab
	case "gitlab_runner":
		return MetricSourceGitlabRunner
	case "glusterfs":
		return MetricSourceGlusterfs
	case "go_expvar":
		return MetricSourceGoExpvar
	case "gpu":
		return MetricSourceGPU
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
	case "kafka_consumer":
		return MetricSourceKafkaConsumer
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
	case "kubevirt_api":
		return MetricSourceKubeVirtAPI
	case "kubevirt_controller":
		return MetricSourceKubeVirtController
	case "kubevirt_handler":
		return MetricSourceKubeVirtHandler
	case "kubelet":
		return MetricSourceKubelet
	case "kubernetes_state":
		return MetricSourceKubernetesState
	case "kyototycoon":
		return MetricSourceKyototycoon
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
	case "openldap":
		return MetricSourceOpenldap
	case "openmetrics":
		return MetricSourceOpenmetrics
	case "openstack":
		return MetricSourceOpenstack
	case "openstack_controller":
		return MetricSourceOpenstackController
	case "oracle":
		return MetricSourceOracle
	case "pdh_check":
		return MetricSourcePdhCheck
	case "pgbouncer":
		return MetricSourcePgbouncer
	case "php_fpm":
		return MetricSourcePhpFpm
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
	case "silk":
		return MetricSourceSilk
	case "silverstripe_cms":
		return MetricSourceSilverstripeCMS
	case "singlestore":
		return MetricSourceSinglestore
	case "snowflake":
		return MetricSourceSnowflake
	case "sonatype_nexus":
		return MetricSourceSonatypeNexus
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
	case "teradata":
		return MetricSourceTeradata
	case "tls":
		return MetricSourceTLS
	case "tokumx":
		return MetricSourceTokumx
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
	case "vllm":
		return MetricSourceVllm
	case "voltdb":
		return MetricSourceVoltdb
	case "vsphere":
		return MetricSourceVsphere
	case "win32_event_log":
		return MetricSourceWin32EventLog
	case "windows_performance_counters":
		return MetricSourceWindowsPerformanceCounters
	case "windows_service":
		return MetricSourceWindowsService
	case "wmi_check":
		return MetricSourceWmiCheck
	case "yarn":
		return MetricSourceYarn
	case "zk":
		return MetricSourceZk
	case "argo_rollouts":
		return MetricSourceArgoRollouts
	case "argo_workflows":
		return MetricSourceArgoWorkflows
	case "cloudera":
		return MetricSourceCloudera
	case "datadog_cluster_agent":
		return MetricSourceDatadogClusterAgent
	case "dcgm":
		return MetricSourceDcgm
	case "esxi":
		return MetricSourceEsxi
	case "fluxcd":
		return MetricSourceFluxcd
	case "karpenter":
		return MetricSourceKarpenter
	case "nvidia_triton":
		return MetricSourceNvidiaTriton
	case "nvidia_nim":
		return MetricSourceNvidiaNim
	case "ray":
		return MetricSourceRay
	case "strimzi":
		return MetricSourceStrimzi
	case "tekton":
		return MetricSourceTekton
	case "teleport":
		return MetricSourceTeleport
	case "temporal":
		return MetricSourceTemporal
	case "torchserve":
		return MetricSourceTorchserve
	case "weaviate":
		return MetricSourceWeaviate
	case "traefik_mesh":
		return MetricSourceTraefikMesh
	case "kubernetes_cluster_autoscaler":
		return MetricSourceKubernetesClusterAutoscaler
	case "aqua":
		return MetricSourceAqua
	case "aws_pricing":
		return MetricSourceAwsPricing
	case "bind9":
		return MetricSourceBind9
	case "cfssl":
		return MetricSourceCfssl
	case "cloudnatix":
		return MetricSourceCloudnatix
	case "cloudsmith":
		return MetricSourceCloudsmith
	case "cybersixgill_actionable_alerts":
		return MetricSourceCybersixgillActionableAlerts
	case "cyral":
		return MetricSourceCyral
	case "emqx":
		return MetricSourceEmqx
	case "eventstore":
		return MetricSourceEventstore
	case "exim":
		return MetricSourceExim
	case "fiddler":
		return MetricSourceFiddler
	case "filebeat":
		return MetricSourceFilebeat
	case "filemage":
		return MetricSourceFilemage
	case "fluentbit":
		return MetricSourceFluentbit
	case "gatekeeper":
		return MetricSourceGatekeeper
	case "gitea":
		return MetricSourceGitea
	case "gnatsd":
		return MetricSourceGnatsd
	case "gnatsd_streaming":
		return MetricSourceGnatsdStreaming
	case "go_pprof_scraper":
		return MetricSourceGoPprofScraper
	case "grpc_check":
		return MetricSourceGrpcCheck
	case "hikaricp":
		return MetricSourceHikaricp
	case "jfrog_platform_self_hosted":
		return MetricSourceJfrogPlatformSelfHosted
	case "kernelcare":
		return MetricSourceKernelcare
	case "lighthouse":
		return MetricSourceLighthouse
	case "logstash":
		return MetricSourceLogstash
	case "mergify":
		return MetricSourceMergify
	case "neo4j":
		return MetricSourceNeo4j
	case "neutrona":
		return MetricSourceNeutrona
	case "nextcloud":
		return MetricSourceNextcloud
	case "nn_sdwan":
		return MetricSourceNnSdwan
	case "ns1":
		return MetricSourceNs1
	case "nvml":
		return MetricSourceNvml
	case "octoprint":
		return MetricSourceOctoprint
	case "open_policy_agent":
		return MetricSourceOpenPolicyAgent
	case "php_apcu":
		return MetricSourcePhpApcu
	case "php_opcache":
		return MetricSourcePhpOpcache
	case "pihole":
		return MetricSourcePihole
	case "ping":
		return MetricSourcePing
	case "portworx":
		return MetricSourcePortworx
	case "puma":
		return MetricSourcePuma
	case "purefa":
		return MetricSourcePurefa
	case "purefb":
		return MetricSourcePurefb
	case "radarr":
		return MetricSourceRadarr
	case "reboot_required":
		return MetricSourceRebootRequired
	case "redis_sentinel":
		return MetricSourceRedisSentinel
	case "redisenterprise":
		return MetricSourceRedisenterprise
	case "redpanda":
		return MetricSourceRedpanda
	case "riak_repl":
		return MetricSourceRiakRepl
	case "scalr":
		return MetricSourceScalr
	case "sendmail":
		return MetricSourceSendmail
	case "snmpwalk":
		return MetricSourceSnmpwalk
	case "sonarr":
		return MetricSourceSonarr
	case "sortdb":
		return MetricSourceSortdb
	case "speedtest":
		return MetricSourceSpeedtest
	case "stardog":
		return MetricSourceStardog
	case "storm":
		return MetricSourceStorm
	case "syncthing":
		return MetricSourceSyncthing
	case "tidb":
		return MetricSourceTidb
	case "traefik":
		return MetricSourceTraefik
	case "unbound":
		return MetricSourceUnbound
	case "unifi_console":
		return MetricSourceUnifiConsole
	case "upbound_uxp":
		return MetricSourceUpboundUxp
	case "upsc":
		return MetricSourceUpsc
	case "vespa":
		return MetricSourceVespa
	case "wayfinder":
		return MetricSourceWayfinder
	case "zabbix":
		return MetricSourceZabbix
	case "zenoh_router":
		return MetricSourceZenohRouter
	case "aws_neuron":
		return MetricSourceAwsNeuron
	case "kyverno":
		return MetricSourceKyverno
	case "anyscale":
		return MetricSourceAnyscale
	case "appgate_sdp":
		return MetricSourceAppgateSDP
	case "slurm":
		return MetricSourceSlurm
	case "tibco_ems":
		return MetricSourceTibcoEMS
	case "milvus":
		return MetricSourceMilvus
	case "quarkus":
		return MetricSourceQuarkus
	case "velero":
		return MetricSourceVelero
	case "altair_pbs_pro":
		return MetricSourceAltairPBSPro
	case "falco":
		return MetricSourceFalco
	case "krakend":
		return MetricSourceKrakenD
	case "kuma":
		return MetricSourceKuma
	case "lite_llm":
		return MetricSourceLiteLLM
	case "lustre":
		return MetricSourceLustre
	case "proxmox":
		return MetricSourceProxmox
	case "resilience4j":
		return MetricSourceResilience4j
	case "supabase":
		return MetricSourceSupabase
	case "keda":
		return MetricSourceKeda
	case "duckdb":
		return MetricSourceDuckdb
	case "bentoml":
		return MetricSourceBentoMl
	case "hugging_face_tgi":
		return MetricSourceHuggingFaceTgi
	case "ibm_spectrum_lsf":
		return MetricSourceIbmSpectrumLsf
	case "datadog_operator":
		return MetricSourceDatadogOperator
	case "opentelemetry_collector_unknown":
		return MetricSourceOpenTelemetryCollectorUnknown
	case "opentelemetry_collector_dockerstatsreceiver":
		return MetricSourceOpenTelemetryCollectorDockerstatsReceiver
	case "opentelemetry_collector_elasticsearchreceiver":
		return MetricSourceOpenTelemetryCollectorElasticsearchReceiver
	case "opentelemetry_collector_expvarreceiver":
		return MetricSourceOpenTelemetryCollectorExpvarReceiver
	case "opentelemetry_collector_filestatsreceiver":
		return MetricSourceOpenTelemetryCollectorFilestatsReceiver
	case "opentelemetry_collector_flinkmetricsreceiver":
		return MetricSourceOpenTelemetryCollectorFlinkmetricsReceiver
	case "opentelemetry_collector_gitproviderreceiver":
		return MetricSourceOpenTelemetryCollectorGitproviderReceiver
	case "opentelemetry_collector_haproxyreceiver":
		return MetricSourceOpenTelemetryCollectorHaproxyReceiver
	case "opentelemetry_collector_hostmetricsreceiver":
		return MetricSourceOpenTelemetryCollectorHostmetricsReceiver
	case "opentelemetry_collector_httpcheckreceiver":
		return MetricSourceOpenTelemetryCollectorHttpcheckReceiver
	case "opentelemetry_collector_iisreceiver":
		return MetricSourceOpenTelemetryCollectorIisReceiver
	case "opentelemetry_collector_k8sclusterreceiver":
		return MetricSourceOpenTelemetryCollectorK8sclusterReceiver
	case "opentelemetry_collector_kafkametricsreceiver":
		return MetricSourceOpenTelemetryCollectorKafkametricsReceiver
	case "opentelemetry_collector_kubeletstatsreceiver":
		return MetricSourceOpenTelemetryCollectorKubeletstatsReceiver
	case "opentelemetry_collector_memcachedreceiver":
		return MetricSourceOpenTelemetryCollectorMemcachedReceiver
	case "opentelemetry_collector_mongodbatlasreceiver":
		return MetricSourceOpenTelemetryCollectorMongodbatlasReceiver
	case "opentelemetry_collector_mongodbreceiver":
		return MetricSourceOpenTelemetryCollectorMongodbReceiver
	case "opentelemetry_collector_mysqlreceiver":
		return MetricSourceOpenTelemetryCollectorMysqlReceiver
	case "opentelemetry_collector_nginxreceiver":
		return MetricSourceOpenTelemetryCollectorNginxReceiver
	case "opentelemetry_collector_nsxtreceiver":
		return MetricSourceOpenTelemetryCollectorNsxtReceiver
	case "opentelemetry_collector_oracledbreceiver":
		return MetricSourceOpenTelemetryCollectorOracledbReceiver
	case "opentelemetry_collector_postgresqlreceiver":
		return MetricSourceOpenTelemetryCollectorPostgresqlReceiver
	case "opentelemetry_collector_prometheusreceiver":
		return MetricSourceOpenTelemetryCollectorPrometheusReceiver
	case "opentelemetry_collector_rabbitmqreceiver":
		return MetricSourceOpenTelemetryCollectorRabbitmqReceiver
	case "opentelemetry_collector_redisreceiver":
		return MetricSourceOpenTelemetryCollectorRedisReceiver
	case "opentelemetry_collector_riakreceiver":
		return MetricSourceOpenTelemetryCollectorRiakReceiver
	case "opentelemetry_collector_saphanareceiver":
		return MetricSourceOpenTelemetryCollectorSaphanaReceiver
	case "opentelemetry_collector_snmpreceiver":
		return MetricSourceOpenTelemetryCollectorSnmpReceiver
	case "opentelemetry_collector_snowflakereceiver":
		return MetricSourceOpenTelemetryCollectorSnowflakeReceiver
	case "opentelemetry_collector_splunkenterprisereceiver":
		return MetricSourceOpenTelemetryCollectorSplunkenterpriseReceiver
	case "opentelemetry_collector_sqlserverreceiver":
		return MetricSourceOpenTelemetryCollectorSqlserverReceiver
	case "opentelemetry_collector_sshcheckreceiver":
		return MetricSourceOpenTelemetryCollectorSshcheckReceiver
	case "opentelemetry_collector_statsdreceiver":
		return MetricSourceOpenTelemetryCollectorStatsdReceiver
	case "opentelemetry_collector_vcenterreceiver":
		return MetricSourceOpenTelemetryCollectorVcenterReceiver
	case "opentelemetry_collector_zookeeperreceiver":
		return MetricSourceOpenTelemetryCollectorZookeeperReceiver
	case "opentelemetry_collector_activedirectorydsreceiver":
		return MetricSourceOpenTelemetryCollectorActiveDirectorydsReceiver
	case "opentelemetry_collector_aerospikereceiver":
		return MetricSourceOpenTelemetryCollectorAerospikeReceiver
	case "opentelemetry_collector_apachereceiver":
		return MetricSourceOpenTelemetryCollectorApacheReceiver
	case "opentelemetry_collector_apachesparkreceiver":
		return MetricSourceOpenTelemetryCollectorApachesparkReceiver
	case "opentelemetry_collector_azuremonitorreceiver":
		return MetricSourceOpenTelemetryCollectorAzuremonitorReceiver
	case "opentelemetry_collector_bigipreceiver":
		return MetricSourceOpenTelemetryCollectorBigipReceiver
	case "opentelemetry_collector_chronyreceiver":
		return MetricSourceOpenTelemetryCollectorChronyReceiver
	case "opentelemetry_collector_couchdbreceiver":
		return MetricSourceOpenTelemetryCollectorCouchdbReceiver
	case "wlan":
		return MetricSourceWlan
	case "windows_certificate":
		return MetricSourceWindowsCertificateStore
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
