// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"reflect"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func TestCorrelationMemberSeriesIDsUseMemberHandles(t *testing.T) {
	first := observerdef.QueryHandle{Ref: 1, Aggregate: observerdef.AggregateAverage}
	second := observerdef.QueryHandle{Ref: 2, Aggregate: observerdef.AggregateAverage}
	descriptor := observerdef.SeriesDescriptor{Name: "cpu", Aggregate: observerdef.AggregateAverage}

	got := correlationMemberSeriesIDs(observerdef.ActiveCorrelation{
		Members:       []observerdef.SeriesDescriptor{descriptor, descriptor},
		MemberHandles: []*observerdef.QueryHandle{&first, &second},
	})
	want := []string{"1:avg", "2:avg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("correlationMemberSeriesIDs() = %v, want %v", got, want)
	}
}
