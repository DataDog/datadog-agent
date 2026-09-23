// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func TestFormatSeriesDescriptorMatchesKey(t *testing.T) {
	descriptors := []observerdef.SeriesDescriptor{
		{},
		{Namespace: "metrics", Name: "cpu", Aggregate: observerdef.AggregateAverage},
		{
			Namespace: "metrics",
			Name:      "cpu",
			Host:      "agent-a",
			Tags:      []string{"team:agent", "env:prod", "env:prod"},
			Aggregate: observerdef.AggregateSum,
		},
	}

	for _, descriptor := range descriptors {
		if got, want := formatSeriesDescriptor(descriptor), descriptor.Key(); got != want {
			t.Errorf("formatSeriesDescriptor(%#v) = %q, want %q", descriptor, got, want)
		}
	}
}

func TestSeriesDescriptorsEqualIgnoresTagOrder(t *testing.T) {
	left := observerdef.SeriesDescriptor{
		Namespace: "metrics",
		Name:      "cpu",
		Host:      "agent-a",
		Tags:      []string{"team:agent", "env:prod", "env:prod"},
		Aggregate: observerdef.AggregateAverage,
	}
	right := left
	right.Tags = []string{"env:prod", "team:agent", "env:prod"}

	if !seriesDescriptorsEqual(left, right) {
		t.Fatal("descriptors with the same tags in a different order must be equal")
	}
	right.Tags = []string{"env:prod", "team:agent"}
	if seriesDescriptorsEqual(left, right) {
		t.Fatal("descriptors with different tag multiplicities must differ")
	}
}

func TestCompareSeriesDescriptorsMatchesLegacyKeyOrder(t *testing.T) {
	descriptors := []observerdef.SeriesDescriptor{
		{Namespace: "metrics", Name: "cpu", Tags: []string{"team:agent", "env:prod"}},
		{Namespace: "metrics", Name: "cpu", Tags: []string{"env:prod", "team:agent"}},
		{Namespace: "metrics", Name: "memory"},
		{Namespace: "logs", Name: "cpu", Aggregate: observerdef.AggregateSum},
	}

	for _, left := range descriptors {
		for _, right := range descriptors {
			got := compareSeriesDescriptors(left, right)
			want := strings.Compare(left.Key(), right.Key())
			if got != want {
				t.Errorf("compareSeriesDescriptors(%#v, %#v) = %d, want %d", left, right, got, want)
			}
		}
	}
}

func TestAnomalyFingerprintPreservesLegacyFormat(t *testing.T) {
	anomaly := observerdef.Anomaly{
		Source: observerdef.SeriesDescriptor{
			Namespace: "metrics",
			Name:      "cpu",
			Host:      "agent-a",
			Tags:      []string{"team:agent", "env:prod"},
			Aggregate: observerdef.AggregateSum,
		},
		Timestamp: 100,
		Title:     "spike",
	}

	if got, want := anomalyFingerprint(anomaly), "metrics|cpu:sum|agent-a|env:prod,team:agent|100|spike"; got != want {
		t.Fatalf("anomalyFingerprint() = %q, want %q", got, want)
	}
}
