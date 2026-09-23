// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func TestFormatSeriesDescriptor(t *testing.T) {
	tests := []struct {
		descriptor observerdef.SeriesDescriptor
		want       string
	}{
		{want: "|:none||"},
		{
			descriptor: observerdef.SeriesDescriptor{Namespace: "metrics", Name: "cpu", Aggregate: observerdef.AggregateAverage},
			want:       "metrics|cpu:avg||",
		},
		{
			descriptor: observerdef.SeriesDescriptor{
				Namespace: "metrics",
				Name:      "cpu",
				Host:      "agent-a",
				Tags:      []string{"team:agent", "env:prod", "env:prod"},
				Aggregate: observerdef.AggregateSum,
			},
			want: "metrics|cpu:sum|agent-a|env:prod,env:prod,team:agent",
		},
	}

	for _, tt := range tests {
		if got := formatSeriesDescriptor(tt.descriptor); got != tt.want {
			t.Errorf("formatSeriesDescriptor(%#v) = %q, want %q", tt.descriptor, got, tt.want)
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
