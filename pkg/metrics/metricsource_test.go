// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricSourceString(t *testing.T) {
	tests := []struct {
		source   MetricSource
		expected string
	}{
		{MetricSourceUnknown, "<unknown>"},
		{MetricSourceDogstatsd, "dogstatsd"},
		{MetricSourceInternal, "internal"},
		{MetricSourceContainer, "container"},
		{MetricSourceDocker, "docker"},
		{MetricSourceKafka, "kafka"},
		{MetricSourceRedisdb, "redisdb"},
		{MetricSourcePostgres, "postgres"},
		{MetricSourceCPU, "cpu"},
		{MetricSourceMemory, "memory"},
		{MetricSourceDisk, "disk"},
		{MetricSourceNetwork, "network"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.source.String())
		})
	}
}

func TestMetricSourceStringUnknown(t *testing.T) {
	// Test that an invalid MetricSource returns "<unknown>"
	invalidSource := MetricSource(65535)
	assert.Equal(t, "<unknown>", invalidSource.String())
}

func TestCheckNameToMetricSource(t *testing.T) {
	tests := []struct {
		name     string
		expected MetricSource
	}{
		// Core checks
		{"container", MetricSourceContainer},
		{"docker", MetricSourceDocker},
		{"ntp", MetricSourceNTP},
		{"cpu", MetricSourceCPU},
		{"memory", MetricSourceMemory},
		{"disk", MetricSourceDisk},
		{"network", MetricSourceNetwork},
		{"snmp", MetricSourceSnmp},
		{"systemd", MetricSourceSystemd},
		{"uptime", MetricSourceUptime},
		{"io", MetricSourceIo},
		// Python checks
		{"redisdb", MetricSourceRedisdb},
		{"postgres", MetricSourcePostgres},
		// telemetry maps to internal
		{"telemetry", MetricSourceInternal},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, CheckNameToMetricSource(tc.name))
		})
	}
}

func TestCheckNameToMetricSourceUnknown(t *testing.T) {
	assert.Equal(t, MetricSourceUnknown, CheckNameToMetricSource("unknown_check_name"))
	assert.Equal(t, MetricSourceUnknown, CheckNameToMetricSource(""))
}

func TestJMXCheckNameToMetricSource(t *testing.T) {
	tests := []struct {
		name     string
		expected MetricSource
	}{
		{"activemq", MetricSourceActivemq},
		{"cassandra", MetricSourceCassandra},
		{"confluent_platform", MetricSourceConfluentPlatform},
		{"hazelcast", MetricSourceHazelcast},
		{"hive", MetricSourceHive},
		{"hivemq", MetricSourceHivemq},
		{"hudi", MetricSourceHudi},
		{"ignite", MetricSourceIgnite},
		{"jboss_wildfly", MetricSourceJbossWildfly},
		{"kafka", MetricSourceKafka},
		{"presto", MetricSourcePresto},
		{"solr", MetricSourceSolr},
		{"sonarqube", MetricSourceSonarqube},
		{"tomcat", MetricSourceTomcat},
		{"weblogic", MetricSourceWeblogic},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, JMXCheckNameToMetricSource(tc.name))
		})
	}
}

func TestJMXCheckNameToMetricSourceCustom(t *testing.T) {
	// Unknown JMX check names should return MetricSourceJmxCustom
	assert.Equal(t, MetricSourceJmxCustom, JMXCheckNameToMetricSource("custom_jmx_check"))
	assert.Equal(t, MetricSourceJmxCustom, JMXCheckNameToMetricSource(""))
}
