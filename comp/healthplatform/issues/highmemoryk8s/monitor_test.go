// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package highmemoryk8s

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
)

func TestNewMonitor(t *testing.T) {
	hostname, _ := hostnamemock.NewMock("node-1")
	m := NewMonitor(storemock.New(t), hostname, logmock.New(t))

	assert.Equal(t, usageMetric, m.Metric)
	assert.Equal(t, limitMetric, m.DivideBy)
	assert.Equal(t, map[string]string{"kind": "agent", "kube_container_name": "agent"}, m.Tags)
	assert.Equal(t, metricsevaluator.ComparatorAbove, m.Comparator)
	assert.Equal(t, 0.9, m.Threshold)
	assert.NotNil(t, m.Callback)
}

func TestMonitorCallback(t *testing.T) {
	store := storemock.New(t)
	hostname, _ := hostnamemock.NewMock("node-1")
	m := NewMonitor(store, hostname, logmock.New(t))
	id := issueID(store, hostname)

	transition := func(previous, state metricsevaluator.State, value float64) {
		m.Callback(metricsevaluator.Event{Monitor: m.Name, Previous: previous, State: state, Value: value})
	}

	// Startup with normal usage: nothing to report or resolve.
	transition(metricsevaluator.StateNoData, metricsevaluator.StateOK, 0.5)
	assert.Nil(t, store.GetIssue(id))
	assert.Empty(t, store.ResolvedIDs())

	// High usage reports the issue with the observed usage.
	transition(metricsevaluator.StateOK, metricsevaluator.StateAlert, 0.934)
	issue := store.GetIssue(id)
	require.NotNil(t, issue)
	assert.Equal(t, id, issue.Id)
	assert.Equal(t, IssueType, issue.IssueType)
	assert.Equal(t, "Datadog Agent memory usage is at 93% of its container limit", issue.Title)

	// A data gap does not resolve the issue.
	transition(metricsevaluator.StateAlert, metricsevaluator.StateNoData, 0)
	assert.NotNil(t, store.GetIssue(id))

	// Usage back down resolves it.
	transition(metricsevaluator.StateAlert, metricsevaluator.StateOK, 0.6)
	assert.Nil(t, store.GetIssue(id))
	assert.Equal(t, []string{id}, store.ResolvedIDs())
}

func TestResolveIssueResolvesEveryActiveID(t *testing.T) {
	// An issue persisted by a previous run (possibly in the older "high-memory-k8s:<reporter>" format), or reported
	// before the DaemonSet uid was resolved, has another id.
	stale := &healthplatform.Issue{Id: IssueIDPrefix + ":0000000000000000", IssueName: IssueName}
	other := &healthplatform.Issue{Id: "other", IssueName: "Other Issue"}
	store := storemock.New(t, storemock.WithIssue(stale), storemock.WithIssue(other))
	hostname, _ := hostnamemock.NewMock("node-1")
	m := NewMonitor(store, hostname, logmock.New(t))

	m.Callback(metricsevaluator.Event{Previous: metricsevaluator.StateNoData, State: metricsevaluator.StateOK, Value: 0.5})

	assert.Nil(t, store.GetIssue(stale.Id), "an issue persisted by a previous run must be resolved at startup")
	assert.NotNil(t, store.GetIssue(other.Id), "issues of other types must be kept")
}

func TestIssueID(t *testing.T) {
	store := storemock.New(t)
	node1, _ := hostnamemock.NewMock("node-1")
	node2, _ := hostnamemock.NewMock("node-2")

	id := issueID(store, node1)
	assert.Regexp(t, "^high-memory-k8s:[0-9a-f]{16}:agent$", id)
	assert.Equal(t, id, issueID(store, node1), "report and resolve must compute the same id")
	// The mock store's discriminator is the hostname; in a DaemonSet every node gets the DaemonSet uid instead.
	assert.NotEqual(t, id, issueID(store, node2))
}
