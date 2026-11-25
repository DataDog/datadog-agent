// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

// ec2InstanceID matches an EC2 instance ID, the host alias that ties the
// otel-agent's hostname to the one the OTLP mapping gives telemetry with an
// EC2 host.id.
var ec2InstanceID = regexp.MustCompile(`^i-[0-9a-f]{8,17}$`)

// TestDogtelHostAliases checks that the otel-agent sends, for the hostname it
// resolves, a host metadata payload with its host aliases and nothing else.
// The cluster runs on an EC2 instance, so the aliases include its instance ID,
// which differs from the <node>-<cluster> hostname.
func (s *dogtelNodefilterTestSuite) TestDogtelHostAliases() {
	if provlocal.IsKindLocal() {
		s.T().Skip("a local kind cluster runs on no cloud provider, so the otel-agent has no host alias to send")
	}
	want := getPodByAppLabel(s, "datadog", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).Spec.NodeName + "-" + dogtelNodefilterClusterName

	// dogtel-standalone.yml has the otel-agent send them every minute.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		payloads, err := intakePayloads(s)
		require.NoError(c, err)
		var aliases []string
		for _, p := range hostAliasesPayloads(payloads) {
			if p.InternalHostname == want {
				assert.Equal(c, want, p.Meta.Hostname)
				aliases = p.Meta.HostAliases
			}
		}
		require.NotEmpty(c, aliases, "no host aliases payload for hostname %q among the /intake/ payloads with the keys %v", want, payloadKeys(payloads))
		assert.NotContains(c, aliases, want, "the hostname itself isn't an alias to send")
		assert.True(c, slices.ContainsFunc(aliases, ec2InstanceID.MatchString), "host aliases %v should include the EC2 instance ID", aliases)
	}, 3*time.Minute, 10*time.Second)
}

// TestDogtelLeavesHostAliasesToCoreAgent checks that the otel-agent sends no
// host aliases while the core Agent, whose pod the Datadog Helm chart labels
// agent.datadoghq.com/component=agent, runs on the same node: that Agent sends
// them with its full host metadata, and the backend would replace one alias
// set with the other.
func (s *dogtelCoexistTestSuite) TestDogtelLeavesHostAliasesToCoreAgent() {
	otelAgentStarted := containerStartedAt(s, getPodByAppLabel(s, "datadog", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]), "otel-agent")
	coreAgentStarted := containerStartedAt(s, getPodByAppLabel(s, coreAgentNamespace, coreAgentPodLabel), "agent")
	// The otel-agent may send host aliases before it sees the core Agent pod,
	// at the latest a sending interval (1 minute, with jitter) after both
	// started, plus the time workloadmeta takes to list the pod.
	since := maxTime(otelAgentStarted, coreAgentStarted).Add(2 * time.Minute)
	// Wait for two more sending intervals, which would bring host aliases if
	// the otel-agent sent them.
	until := since.Add(2 * time.Minute)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		payloads, err := intakePayloads(s)
		require.NoError(c, err)
		assert.True(c, slices.ContainsFunc(payloads, isCoreAgentHostMetadata), "no host metadata from the core Agent among the /intake/ payloads with the keys %v", payloadKeys(payloads))
		assert.True(c, time.Now().After(until), "waiting until %s for the otel-agent to have had two chances to send host aliases", until.Format(time.RFC3339))
	}, 7*time.Minute, 15*time.Second)

	payloads, err := intakePayloads(s)
	require.NoError(s.T(), err)
	var sent []hostAliasesPayload
	for _, p := range hostAliasesPayloads(payloads) {
		if p.received.After(since) {
			sent = append(sent, p)
		}
	}
	assert.Empty(s.T(), sent, "the otel-agent sent host aliases after %s although the core Agent runs on its node", since.Format(time.RFC3339))
}

// intakePayload is a JSON payload fakeintake received on /intake/, where the
// Agent posts host metadata and events alike.
type intakePayload struct {
	received time.Time
	fields   map[string]json.RawMessage
}

func intakePayloads(s utils.OTelTestSuite) ([]intakePayload, error) {
	raw, err := s.Env().FakeIntake.Client().GetRawPayloads("/intake/")
	if err != nil {
		return nil, err
	}
	payloads := make([]intakePayload, 0, len(raw))
	for _, p := range raw {
		inflated, err := aggregator.Inflate(p.Data, p.Encoding)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(inflated, &fields); err != nil {
			return nil, err
		}
		payloads = append(payloads, intakePayload{received: p.Timestamp, fields: fields})
	}
	return payloads, nil
}

// hostAliasesPayload is the payload the dogtel extension sends: the
// otel-agent's hostname and host aliases.
type hostAliasesPayload struct {
	received         time.Time
	InternalHostname string `json:"internalHostname"`
	Meta             struct {
		Hostname    string   `json:"hostname"`
		HostAliases []string `json:"host_aliases"`
	} `json:"meta"`
}

// hostAliasesPayloads returns the payloads shaped like the dogtel extension's:
// internalHostname and meta only, with only hostname and host_aliases in meta.
// The core Agent's host metadata carries many more fields.
func hostAliasesPayloads(payloads []intakePayload) []hostAliasesPayload {
	var found []hostAliasesPayload
	for _, p := range payloads {
		if !hasKeys(p.fields, "internalHostname", "meta") {
			continue
		}
		var meta map[string]json.RawMessage
		if json.Unmarshal(p.fields["meta"], &meta) != nil || !hasKeys(meta, "hostname", "host_aliases") {
			continue
		}
		hap := hostAliasesPayload{received: p.received}
		if json.Unmarshal(p.fields["internalHostname"], &hap.InternalHostname) != nil ||
			json.Unmarshal(p.fields["meta"], &hap.Meta) != nil {
			continue
		}
		found = append(found, hap)
	}
	return found
}

// isCoreAgentHostMetadata reports whether p is the core Agent's host metadata.
func isCoreAgentHostMetadata(p intakePayload) bool {
	_, hasVersion := p.fields["agentVersion"]
	_, hasMeta := p.fields["meta"]
	return hasVersion && hasMeta
}

// hasKeys reports whether the keys of m are exactly keys.
func hasKeys(m map[string]json.RawMessage, keys ...string) bool {
	return len(m) == len(keys) && !slices.ContainsFunc(keys, func(k string) bool {
		_, ok := m[k]
		return !ok
	})
}

// payloadKeys lists the sorted top-level keys of each payload, for failure messages.
func payloadKeys(payloads []intakePayload) [][]string {
	keys := make([][]string, 0, len(payloads))
	for _, p := range payloads {
		keys = append(keys, slices.Sorted(maps.Keys(p.fields)))
	}
	return keys
}

// containerStartedAt returns when containerName of pod last started running.
func containerStartedAt(s utils.OTelTestSuite, pod corev1.Pod, containerName string) time.Time {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == containerName {
			require.NotNil(s.T(), cs.State.Running, "container %s of pod %s isn't running", containerName, pod.Name)
			return cs.State.Running.StartedAt.Time
		}
	}
	require.Failf(s.T(), "container not found", "container %s not found in pod %s", containerName, pod.Name)
	return time.Time{}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
