// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func TestAnomalySourceIdentityForStorageBackedAnomaly(t *testing.T) {
	handle := observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage}
	identity := anomalySourceIdentityFor(observerdef.Anomaly{
		Source:    observerdef.SeriesDescriptor{Name: "ignored-when-ref-is-present"},
		SourceRef: &handle,
	})

	want := anomalySourceIdentity{handle: handle, hasHandle: true}
	if identity != want {
		t.Fatalf("identity = %#v, want %#v", identity, want)
	}
}

func TestAnomalySourceIdentityDistinguishesAggregates(t *testing.T) {
	average := observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage}
	sum := observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateSum}

	averageIdentity := anomalySourceIdentityFor(observerdef.Anomaly{SourceRef: &average})
	sumIdentity := anomalySourceIdentityFor(observerdef.Anomaly{SourceRef: &sum})
	if averageIdentity == sumIdentity {
		t.Fatal("storage-backed identities with different aggregates must differ")
	}
}

func TestAnomalySourceIdentityForAnomalyWithoutStorageSource(t *testing.T) {
	first := anomalySourceIdentityFor(observerdef.Anomaly{Source: observerdef.SeriesDescriptor{
		Namespace: "rrcf",
		Name:      "score",
		Host:      "agent-a",
		Tags:      []string{"env:test", "team:agent"},
	}})
	second := anomalySourceIdentityFor(observerdef.Anomaly{Source: observerdef.SeriesDescriptor{
		Namespace: "rrcf",
		Name:      "score",
		Host:      "agent-a",
		Tags:      []string{"team:agent", "env:test"},
	}})

	if first.hasHandle {
		t.Fatal("ref-less anomaly identity unexpectedly has a storage handle")
	}
	if first.fallback == "" {
		t.Fatal("ref-less anomaly identity has an empty fallback")
	}
	if first != second {
		t.Fatalf("fallback identities differ: %#v and %#v", first, second)
	}
}
