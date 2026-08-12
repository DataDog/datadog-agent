// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/stretchr/testify/assert"
)

func TestTailerScopeMatcherExactAndReferenceCounted(t *testing.T) {
	m := newTailerScopeMatcher(nil, 1000)
	first, second := &sources.LogSource{}, &sources.LogSource{}
	scope := tailerScope{service: "checkout", source: "java"}
	m.register(first, scope)
	m.register(second, scope)
	m.classifyMetric("dogstatsd", &metricObs{tags: []string{"service:checkout", "source:java"}})
	m.remove(first)
	m.classifyMetric("dogstatsd", &metricObs{tags: []string{"service:checkout", "source:java"}})
	m.remove(second)
	m.classifyMetric("dogstatsd", &metricObs{tags: []string{"service:checkout", "source:java"}})

	report := m.SnapshotAndResetTailerMatchReport()
	assert.Equal(t, 2, report.MatchedMetrics)
	assert.Equal(t, 1, report.UnmatchedMetrics)
}

func TestMetricScopeRequiresOneNonEmptyIdentity(t *testing.T) {
	_, _, outcome := metricScope([]string{"service:checkout"})
	assert.Equal(t, "missing_source", outcome)
	_, _, outcome = metricScope([]string{"service:checkout", "source:java", "source:nginx"})
	assert.Equal(t, "ambiguous_identity", outcome)
	_, _, outcome = metricScope([]string{"service:checkout", "service:checkout", "source:java"})
	assert.Empty(t, outcome)
}

func TestScopeFromStringsRejectsPartialScopes(t *testing.T) {
	_, reason := scopeFromStrings("", "java")
	assert.Equal(t, "missing_service", reason)
	_, reason = scopeFromStrings("checkout", "")
	assert.Equal(t, "missing_source", reason)
}
