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

// StandardStatsdPrefixes is a list of the statsd prefixes used by the agent and it's components
var StandardStatsdPrefixes = []string{
	"datadog.trace_agent",
	"datadog.process",
	"datadog.agent",
	"datadog.dogstatsd",
}

func init() {
	// JMXFetch sends data through
	for jmxIntegration := range StandardJMXIntegrations {
		StandardStatsdPrefixes = append(StandardStatsdPrefixes, jmxIntegration)
	}
}
