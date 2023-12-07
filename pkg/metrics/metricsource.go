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
	case "telemetry":
		return MetricSourceInternal
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
