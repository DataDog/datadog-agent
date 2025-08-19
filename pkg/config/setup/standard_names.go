// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

// StandardJMXIntegrations is the list of standard jmx integrations.
// This list is used by the Agent to determine if an integration is JMXFetch-based,
// based only on the integration name.
// DEPRECATED: this list is only used for backward compatibility with older JMXFetch integration
// configs. All JMXFetch integrations should instead define `is_jmx: true` at the init_config or
// instance level.
var StandardJMXIntegrations = map[string]struct{}{
	"activemq":    {},
	"activemq_58": {},
	"cassandra":   {},
	"jmx":         {},
	"presto":      {},
	"solr":        {},
	"tomcat":      {},
	"kafka":       {},
}

// StandardStatsdPrefixes is a list of the statsd prefixes used by the agent and its components
var StandardStatsdPrefixes = []string{
	"datadog.agent",
	"datadog.dogstatsd",
	"datadog.process",
	"datadog.trace_agent",
	"datadog.tracer",

	"activemq",
	"activemq_58",
	"airflow",
	"cassandra",
	"confluent",
	"hazelcast",
	"hive",
	"ignite",
	"jboss",
	"jvm",
	"kafka",
	"presto",
	"sidekiq",
	"solr",
	"tomcat",

	"runtime",
}
