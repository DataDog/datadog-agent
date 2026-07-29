// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build jmx

package collectorimpl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	healthplatformnoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// TestJMXCheckIDMatchesInventoryHash asserts that the check ID emitted in the
// agent_checks payload for JMX checks matches the config.hash format built by
// getJMXChecksMetadata() in integrations_jmx.go, including the common case where
// the instance config does not set an explicit "name:" field.
//
// Regression test for the bug where an unnamed instance produced
// "<check>-unknown-unknown:<jmxfetch-instance-name>" instead of the expected
// "<check>-<host>-<port>".
func TestJMXCheckIDMatchesInventoryHash(t *testing.T) {
	// GIVEN two scheduled JMX configs: one instance WITHOUT an explicit name
	// (the buggy case) and one WITH a name (the regression guard).
	jmxfetch.AddScheduledConfig(integration.Config{
		Name: "cassandra",
		Instances: []integration.Data{
			integration.Data("host: 10.112.199.52\nport: 7199\n"),
		},
	})
	jmxfetch.AddScheduledConfig(integration.Config{
		Name: "kafka",
		Instances: []integration.Data{
			integration.Data("host: localhost\nport: 9999\nname: mykafka\n"),
		},
	})
	// Instance with an explicit but empty name: integrations_jmx.go appends the
	// suffix on key presence, so config.hash ends with a trailing ":".
	jmxfetch.AddScheduledConfig(integration.Config{
		Name: "zookeeper",
		Instances: []integration.Data{
			integration.Data("host: localhost\nport: 2181\nname: \"\"\n"),
		},
	})

	// AND matching JMX runtime status. JMXFetch reports instance_name as the
	// auto-generated "<check>-<host>-<port>" for unnamed instances, and as the
	// configured name for named instances. The ChecksStatus type is unexported,
	// so the status is built via JSON.
	statusJSON := `{
		"checks": {
			"initialized_checks": {
				"cassandra": [
					{"instance_name": "cassandra-10.112.199.52-7199", "status": "OK", "message": ""}
				],
				"kafka": [
					{"instance_name": "mykafka", "status": "OK", "message": ""}
				],
				"zookeeper": [
					{"instance_name": "zookeeper-localhost-2181", "status": "OK", "message": ""}
				]
			}
		}
	}`
	var status jmxStatus.Status
	require.NoError(t, json.Unmarshal([]byte(statusJSON), &status))
	jmxStatus.SetStatus(status)
	t.Cleanup(jmxStatus.ClearStatus)

	hostname, _ := hostnameinterface.NewMock("my-hostname")
	c := newCollector(dependencies{
		Lc:               compdef.NewTestLifecycle(t),
		Config:           config.NewMockWithOverrides(t, map[string]interface{}{"check_cancel_timeout": 500 * time.Millisecond}),
		Log:              logmock.New(t),
		HaAgent:          haagentmock.NewMockHaAgent(),
		HealthPlatform:   healthplatformnoopimpl.NewNoopComponent(),
		Hostname:         hostname,
		SenderManager:    aggregator.NewNoOpSenderManager(),
		MetricSerializer: option.None[serializer.MetricSerializer](),
		AgentTelemetry:   option.None[agenttelemetry.Component](),
	})

	// WHEN building the agent_checks payload
	payload := c.GetPayload(context.Background())

	// THEN each JMX check ID matches the config.hash format from integrations_jmx.go.
	checkIDs := jmxCheckIDsByName(payload)

	// Unnamed instance: no "unknown-unknown", no ":name" suffix.
	require.Contains(t, checkIDs, "cassandra")
	assert.Equal(t, "cassandra-10.112.199.52-7199", checkIDs["cassandra"])

	// Named instance: unchanged behavior, "<check>-<host>-<port>:<name>".
	require.Contains(t, checkIDs, "kafka")
	assert.Equal(t, "kafka-localhost-9999:mykafka", checkIDs["kafka"])

	// Explicit empty name: the "name:" key is present, so the suffix is kept,
	// producing a trailing ":" to match integrations_jmx.go's presence check.
	require.Contains(t, checkIDs, "zookeeper")
	assert.Equal(t, "zookeeper-localhost-2181:", checkIDs["zookeeper"])
}

// jmxCheckIDsByName extracts, from the agent_checks payload, a map of check name
// to check ID (the 3rd tuple element) for the JMX checks under test.
func jmxCheckIDsByName(payload *Payload) map[string]string {
	wanted := map[string]struct{}{"cassandra": {}, "kafka": {}, "zookeeper": {}}
	out := map[string]string{}
	for _, raw := range payload.AgentChecks {
		tuple, ok := raw.([]interface{})
		if !ok || len(tuple) < 3 {
			continue
		}
		name, ok := tuple[0].(string)
		if !ok {
			continue
		}
		if _, want := wanted[name]; !want {
			continue
		}
		if checkID, ok := tuple[2].(string); ok {
			out[name] = checkID
		}
	}
	return out
}
