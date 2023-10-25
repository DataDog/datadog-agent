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
	default:
		return "<unknown>"

	}
}
