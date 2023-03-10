package metrics

// MetricSource represents how this metric made it into the Agent
type MetricSource int

// Enumeration of the existing API metric types
const (
	MetricSourceUnknown MetricSource = iota

	MetricSourceDogstatsd
	MetricSourceJmx

	// Corechecks
	// TODO add all these to protobuf
	MetricSourceContainer
	MetricSourceContainerd
	MetricSourceCri
	MetricSourceDocker
	MetricSourceNtp
	MetricSourceSystemd
	MetricSourceHelm
	MetricSourceKubernetesAPIServer
	MetricSourceKubeStateMetrics
	MetricSourceOrchestrator
	MetricSourceWinProc
	MetricSourceFileHandle
	MetricSourceWinkmem
	MetricSourceIoStats
	MetricSourceUptime
	MetricSourceSbom
	MetricSourceMemory
	MetricSourceTcpQueueLength
	MetricSourceOomKill
	MetricSourceContainerLifecycle
	MetricSourceJetson
	MetricSourceContainerImage
	MetricSourceCpu
	MetricSourceLoad
	// The following are both core checks and python checks.
	// I think this is OK as long as the check IDs are the same
	// TODO - double check that the check names are the same
	// MetricSourceNetwork // 'network'
	// MetricSourceSnmp // 'snmp'
	// MetricSourceDisk // 'disk'

	// Python integrations
	MetricSourceActiveDirectory
	MetricSourceActivemqXml
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
	MetricSourceCiscoAci
	MetricSourceCitrixHypervisor
	MetricSourceClickhouse
	MetricSourceCloudFoundryApi
	MetricSourceCockroachdb
	MetricSourceConsul
	MetricSourceCoredns
	MetricSourceCouch
	MetricSourceCouchbase
	MetricSourceCrio
	MetricSourceDirectory
	MetricSourceDisk
	MetricSourceDnsCheck
	MetricSourceDotnetclr
	MetricSourceDruid
	MetricSourceEcsFargate
	MetricSourceEksFargate
	MetricSourceElastic
	MetricSourceEnvoy
	MetricSourceEtcd
	MetricSourceExchangeServer
	MetricSourceExternalDns
	MetricSourceFluentd
	MetricSourceFoundationdb
	MetricSourceGearmand
	MetricSourceGitlabRunner
	MetricSourceGitlab
	MetricSourceGlusterfs
	MetricSourceGoExpvar
	MetricSourceGunicorn
	MetricSourceHaproxy
	MetricSourceHarbor
	MetricSourceHdfsDatanode
	MetricSourceHdfsNamenode
	MetricSourceHttpCheck
	MetricSourceHyperv
	MetricSourceIbmAce
	MetricSourceIbmDb
	MetricSourceIbmI
	MetricSourceIbmMq
	MetricSourceIbmWas
	MetricSourceIis
	MetricSourceImpala
	MetricSourceIstio
	MetricSourceKafkaConsumer
	MetricSourceKong
	MetricSourceKubeApiserverMetrics
	MetricSourceKubeControllerManager
	MetricSourceKubeDns
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
	MetricSourceNetwork
	MetricSourceNfsstat
	MetricSourceNginxIngressController
	MetricSourceNginx
	MetricSourceOpenldap
	MetricSourceOpenmetrics
	MetricSourceOpenstackController
	MetricSourceOpenstack
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
	MetricSourceSnmp
	MetricSourceSnowflake
	MetricSourceSpark
	MetricSourceSqlserver
	MetricSourceSquid
	MetricSourceSshCheck
	MetricSourceStatsd
	MetricSourceSupervisord
	MetricSourceSystemCore
	MetricSourceSystemSwap
	MetricSourceTcpCheck
	MetricSourceTeamcity
	MetricSourceTeradata
	MetricSourceTls
	MetricSourceTokumx
	MetricSourceTrafficServer
	MetricSourceTwemproxy
	MetricSourceTwistlock
	MetricSourceVarnish
	MetricSourceVault
	MetricSourceVertica
	MetricSourceVoltdb
	MetricSourceVsphere
	MetricSourceWinEventLog
	MetricSourceWindowsPerformanceCounters
	MetricSourceWindowsService
	MetricSourceWmiCheck
	MetricSourceYarn
	MetricSourceZk
)

func CheckNameToMetricSource(checkName string) MetricSource {
	switch checkName {
	// Start Manually Specified Block
	case "system", "io", "load", "cpu", "memory", "uptime", "file_handle":
		return MetricSourceSystemCore
	case "docker":
		return MetricSourceDocker
	case "container":
		return MetricSourceContainer
	case "ntp":
		return MetricSourceNtp
	// End Manually SpecifiedBlock
	case "active_directory":
		return MetricSourceActiveDirectory
	case "activemq_xml":
		return MetricSourceActivemqXml
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
	case "cisco_aci":
		return MetricSourceCiscoAci
	case "citrix_hypervisor":
		return MetricSourceCitrixHypervisor
	case "clickhouse":
		return MetricSourceClickhouse
	case "cloud_foundry_api":
		return MetricSourceCloudFoundryApi
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
	case "disk":
		return MetricSourceDisk
	case "dns_check":
		return MetricSourceDnsCheck
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
		return MetricSourceExternalDns
	case "fluentd":
		return MetricSourceFluentd
	case "foundationdb":
		return MetricSourceFoundationdb
	case "gearmand":
		return MetricSourceGearmand
	case "gitlab_runner":
		return MetricSourceGitlabRunner
	case "gitlab":
		return MetricSourceGitlab
	case "glusterfs":
		return MetricSourceGlusterfs
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
		return MetricSourceHttpCheck
	case "hyperv":
		return MetricSourceHyperv
	case "ibm_ace":
		return MetricSourceIbmAce
	case "ibm_db2":
		return MetricSourceIbmDb
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
		return MetricSourceKubeApiserverMetrics
	case "kube_controller_manager":
		return MetricSourceKubeControllerManager
	case "kube_dns":
		return MetricSourceKubeDns
	case "kube_metrics_server":
		return MetricSourceKubeMetricsServer
	case "kube_proxy":
		return MetricSourceKubeProxy
	case "kube_scheduler":
		return MetricSourceKubeScheduler
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
	case "network":
		return MetricSourceNetwork
	case "nfsstat":
		return MetricSourceNfsstat
	case "nginx_ingress_controller":
		return MetricSourceNginxIngressController
	case "nginx":
		return MetricSourceNginx
	case "openldap":
		return MetricSourceOpenldap
	case "openmetrics":
		return MetricSourceOpenmetrics
	case "openstack_controller":
		return MetricSourceOpenstackController
	case "openstack":
		return MetricSourceOpenstack
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
	case "singlestore":
		return MetricSourceSinglestore
	case "snmp":
		return MetricSourceSnmp
	case "snowflake":
		return MetricSourceSnowflake
	case "spark":
		return MetricSourceSpark
	case "sqlserver":
		return MetricSourceSqlserver
	case "squid":
		return MetricSourceSquid
	case "ssh_check":
		return MetricSourceSshCheck
	case "statsd":
		return MetricSourceStatsd
	case "supervisord":
		return MetricSourceSupervisord
	case "system_core":
		return MetricSourceSystemCore
	case "system_swap":
		return MetricSourceSystemSwap
	case "tcp_check":
		return MetricSourceTcpCheck
	case "teamcity":
		return MetricSourceTeamcity
	case "teradata":
		return MetricSourceTeradata
	case "tls":
		return MetricSourceTls
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
	case "voltdb":
		return MetricSourceVoltdb
	case "vsphere":
		return MetricSourceVsphere
	case "win32_event_log":
		return MetricSourceWinEventLog
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
	}

	return MetricSourceUnknown
}

// String returns a string representation of APIMetricType
func (ms MetricSource) String() string {
	switch ms {
	case MetricSourceDogstatsd:
		return "dogstatsd"
	// Corechecks
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
	case MetricSourceKubeStateMetrics:
		return "kubernetes_state_core"
	case MetricSourceOrchestrator:
		return "orchestrator"
	case MetricSourceWinProc:
		return "winproc"
	case MetricSourceFileHandle:
		return "file_handle"
	case MetricSourceWinkmem:
		return "winkmem"
	case MetricSourceIoStats:
		return "io"
	case MetricSourceUptime:
		return "uptime"
	case MetricSourceSbom:
		return "sbom"
	case MetricSourceMemory:
		return "memory"
	case MetricSourceTcpQueueLength:
		return "tcp_queue_length"
	case MetricSourceOomKill:
		return "oom_kill"
	case MetricSourceContainerLifecycle:
		return "container_lifecycle"
	case MetricSourceJetson:
		return "jetson"
	case MetricSourceContainerImage:
		return "container_image"
	case MetricSourceCpu:
		return "cpu"
	case MetricSourceLoad:
		return "load"

	// Python checks
	case MetricSourceActiveDirectory:
		return "active_directory"
	case MetricSourceActivemqXml:
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
	case MetricSourceCiscoAci:
		return "cisco_aci"
	case MetricSourceCitrixHypervisor:
		return "citrix_hypervisor"
	case MetricSourceClickhouse:
		return "clickhouse"
	case MetricSourceCloudFoundryApi:
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
	case MetricSourceDisk:
		return "disk"
	case MetricSourceDnsCheck:
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
	case MetricSourceExternalDns:
		return "external_dns"
	case MetricSourceFluentd:
		return "fluentd"
	case MetricSourceFoundationdb:
		return "foundationdb"
	case MetricSourceGearmand:
		return "gearmand"
	case MetricSourceGitlabRunner:
		return "gitlab_runner"
	case MetricSourceGitlab:
		return "gitlab"
	case MetricSourceGlusterfs:
		return "glusterfs"
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
	case MetricSourceHttpCheck:
		return "http_check"
	case MetricSourceHyperv:
		return "hyperv"
	case MetricSourceIbmAce:
		return "ibm_ace"
	case MetricSourceIbmDb:
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
	case MetricSourceKafkaConsumer:
		return "kafka_consumer"
	case MetricSourceKong:
		return "kong"
	case MetricSourceKubeApiserverMetrics:
		return "kube_apiserver_metrics"
	case MetricSourceKubeControllerManager:
		return "kube_controller_manager"
	case MetricSourceKubeDns:
		return "kube_dns"
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
	case MetricSourceNetwork:
		return "network"
	case MetricSourceNfsstat:
		return "nfsstat"
	case MetricSourceNginxIngressController:
		return "nginx_ingress_controller"
	case MetricSourceNginx:
		return "nginx"
	case MetricSourceOpenldap:
		return "openldap"
	case MetricSourceOpenmetrics:
		return "openmetrics"
	case MetricSourceOpenstackController:
		return "openstack_controller"
	case MetricSourceOpenstack:
		return "openstack"
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
	case MetricSourceSnmp:
		return "snmp"
	case MetricSourceSnowflake:
		return "snowflake"
	case MetricSourceSpark:
		return "spark"
	case MetricSourceSqlserver:
		return "sqlserver"
	case MetricSourceSquid:
		return "squid"
	case MetricSourceSshCheck:
		return "ssh_check"
	case MetricSourceStatsd:
		return "statsd"
	case MetricSourceSupervisord:
		return "supervisord"
	case MetricSourceSystemCore:
		return "system_core"
	case MetricSourceSystemSwap:
		return "system_swap"
	case MetricSourceTcpCheck:
		return "tcp_check"
	case MetricSourceTeamcity:
		return "teamcity"
	case MetricSourceTeradata:
		return "teradata"
	case MetricSourceTls:
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
	case MetricSourceVoltdb:
		return "voltdb"
	case MetricSourceVsphere:
		return "vsphere"
	case MetricSourceWinEventLog:
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
	default:
		return "<unknown>"
	}
}

func (ms MetricSource) OriginCategory() int32 {
	// Constants from `origin.proto`
	switch ms {
	case MetricSourceDogstatsd:
		return 10
	default:
		// integration
		return 11
	}
}

func (ms MetricSource) OriginService() int32 {
	// Constants from `origin.proto`
	switch ms {
	case MetricSourceDogstatsd:
		return 0 // no service
	case MetricSourceActiveDirectory:
		return 10
	case MetricSourceActivemqXml:
		return 11
	case MetricSourceAerospike:
		return 13
	case MetricSourceAirflow:
		return 14
	case MetricSourceAmazonMsk:
		return 15
	case MetricSourceAmbari:
		return 16
	case MetricSourceApache:
		return 17
	case MetricSourceArangodb:
		return 18
	case MetricSourceArgocd:
		return 19
	case MetricSourceAspdotnet:
		return 20
	case MetricSourceAviVantage:
		return 21
	case MetricSourceAzureIotEdge:
		return 22
	case MetricSourceBoundary:
		return 23
	case MetricSourceBtrfs:
		return 24
	case MetricSourceCacti:
		return 25
	case MetricSourceCalico:
		return 26
	case MetricSourceCassandraNodetool:
		return 27
	case MetricSourceCeph:
		return 29
	case MetricSourceCertManager:
		return 30
	case MetricSourceCilium:
		return 34
	case MetricSourceCiscoAci:
		return 35
	case MetricSourceCitrixHypervisor:
		return 36
	case MetricSourceClickhouse:
		return 37
	case MetricSourceCloudFoundryApi:
		return 38
	case MetricSourceCockroachdb:
		return 39
	case MetricSourceConsul:
		return 41
	case MetricSourceCoredns:
		return 42
	case MetricSourceCouch:
		return 43
	case MetricSourceCouchbase:
		return 44
	case MetricSourceCrio:
		return 45
	case MetricSourceDirectory:
		return 47
	case MetricSourceDisk:
		return 48
	case MetricSourceDnsCheck:
		return 49
	case MetricSourceDotnetclr:
		return 50
	case MetricSourceDruid:
		return 51
	case MetricSourceEcsFargate:
		return 52
	case MetricSourceEksFargate:
		return 53
	case MetricSourceElastic:
		return 54
	case MetricSourceEnvoy:
		return 55
	case MetricSourceEtcd:
		return 56
	case MetricSourceExchangeServer:
		return 57
	case MetricSourceExternalDns:
		return 58
	case MetricSourceFluentd:
		return 60
	case MetricSourceFoundationdb:
		return 61
	case MetricSourceGearmand:
		return 62
	case MetricSourceGitlabRunner:
		return 63
	case MetricSourceGitlab:
		return 64
	case MetricSourceGlusterfs:
		return 65
	case MetricSourceGoExpvar:
		return 66
	case MetricSourceGunicorn:
		return 67
	case MetricSourceHaproxy:
		return 68
	case MetricSourceHarbor:
		return 69
	case MetricSourceHdfsDatanode:
		return 71
	case MetricSourceHdfsNamenode:
		return 72
	case MetricSourceHttpCheck:
		return 75
	case MetricSourceHyperv:
		return 77
	case MetricSourceIbmAce:
		return 78
	case MetricSourceIbmDb:
		return 79
	case MetricSourceIbmI:
		return 80
	case MetricSourceIbmMq:
		return 81
	case MetricSourceIbmWas:
		return 82
	case MetricSourceIis:
		return 84
	case MetricSourceImpala:
		return 85
	case MetricSourceIstio:
		return 86
	case MetricSourceKafkaConsumer:
		return 89
	case MetricSourceKong:
		return 91
	case MetricSourceKubeApiserverMetrics:
		return 92
	case MetricSourceKubeControllerManager:
		return 93
	case MetricSourceKubeDns:
		return 94
	case MetricSourceKubeMetricsServer:
		return 95
	case MetricSourceKubeProxy:
		return 96
	case MetricSourceKubeScheduler:
		return 97
	case MetricSourceKubelet:
		return 98
	case MetricSourceKubernetesState:
		return 99
	case MetricSourceKyototycoon:
		return 100
	case MetricSourceLighttpd:
		return 101
	case MetricSourceLinkerd:
		return 102
	case MetricSourceLinuxProcExtras:
		return 103
	case MetricSourceMapr:
		return 104
	case MetricSourceMapreduce:
		return 105
	case MetricSourceMarathon:
		return 106
	case MetricSourceMarklogic:
		return 107
	case MetricSourceMcache:
		return 108
	case MetricSourceMesosMaster:
		return 109
	case MetricSourceMesosSlave:
		return 110
	case MetricSourceMongo:
		return 111
	case MetricSourceMysql:
		return 112
	case MetricSourceNagios:
		return 113
	case MetricSourceNetwork:
		return 114
	case MetricSourceNfsstat:
		return 115
	case MetricSourceNginxIngressController:
		return 116
	case MetricSourceNginx:
		return 117
	case MetricSourceOpenldap:
		return 118
	case MetricSourceOpenmetrics:
		return 119
	case MetricSourceOpenstackController:
		return 120
	case MetricSourceOpenstack:
		return 121
	case MetricSourceOracle:
		return 122
	case MetricSourcePdhCheck:
		return 124
	case MetricSourcePgbouncer:
		return 125
	case MetricSourcePhpFpm:
		return 126
	case MetricSourcePostfix:
		return 127
	case MetricSourcePostgres:
		return 128
	case MetricSourcePowerdnsRecursor:
		return 129
	case MetricSourceProcess:
		return 131
	case MetricSourcePrometheus:
		return 132
	case MetricSourceProxysql:
		return 133
	case MetricSourcePulsar:
		return 134
	case MetricSourceRabbitmq:
		return 135
	case MetricSourceRedisdb:
		return 136
	case MetricSourceRethinkdb:
		return 137
	case MetricSourceRiak:
		return 138
	case MetricSourceRiakcs:
		return 139
	case MetricSourceSapHana:
		return 140
	case MetricSourceScylla:
		return 141
	case MetricSourceSilk:
		return 143
	case MetricSourceSinglestore:
		return 144
	case MetricSourceSnmp:
		return 145
	case MetricSourceSnowflake:
		return 146
	case MetricSourceSpark:
		return 149
	case MetricSourceSqlserver:
		return 150
	case MetricSourceSquid:
		return 151
	case MetricSourceSshCheck:
		return 152
	case MetricSourceStatsd:
		return 153
	case MetricSourceSupervisord:
		return 154
	case MetricSourceSystemCore:
		return 155
	case MetricSourceSystemSwap:
		return 156
	case MetricSourceTcpCheck:
		return 157
	case MetricSourceTeamcity:
		return 158
	case MetricSourceTeradata:
		return 160
	case MetricSourceTls:
		return 161
	case MetricSourceTokumx:
		return 162
	case MetricSourceTrafficServer:
		return 164
	case MetricSourceTwemproxy:
		return 165
	case MetricSourceTwistlock:
		return 166
	case MetricSourceVarnish:
		return 167
	case MetricSourceVault:
		return 168
	case MetricSourceVertica:
		return 169
	case MetricSourceVoltdb:
		return 170
	case MetricSourceVsphere:
		return 171
	case MetricSourceWinEventLog:
		return 173
	case MetricSourceWindowsPerformanceCounters:
		return 174
	case MetricSourceWindowsService:
		return 175
	case MetricSourceWmiCheck:
		return 176
	case MetricSourceYarn:
		return 177
	case MetricSourceZk:
		return 178
	case MetricSourceContainer:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceContainerd:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceCri:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceDocker:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceNtp:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceSystemd:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceHelm:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceKubernetesAPIServer:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceKubeStateMetrics:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceOrchestrator:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceWinProc:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceFileHandle:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceWinkmem:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceIoStats:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceUptime:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceSbom:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceMemory:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceTcpQueueLength:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceOomKill:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceContainerLifecycle:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceJetson:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceContainerImage:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceCpu:
		return -1 // TODO fill in appropriate protobuf value
	case MetricSourceLoad:
		return -1 // TODO fill in appropriate protobuf value
	default:
		return -1
	}
}
