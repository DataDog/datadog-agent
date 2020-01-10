package config

// StandardJMXIntegrations is the list of standard jmx integrations
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
	"cassandra",
	"jvm",
	"presto",
	"solr",
	"tomcat",
	"kafka",
	"runtime",
}
