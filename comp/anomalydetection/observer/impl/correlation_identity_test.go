// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/require"
)

func correlationFixture(n int) []observer.Anomaly {
	anomalies := make([]observer.Anomaly, 0, n*3)
	for i := 0; i < n; i++ {
		for repeat := 0; repeat < 3; repeat++ {
			tags := []string{"a:1", "b:2", "c:3", "d:4", "e:5", "f:6", "g:7", "h:8"}
			if repeat%2 != 0 {
				tags[0], tags[7] = tags[7], tags[0]
			}
			anomalies = append(anomalies, observer.Anomaly{Source: observer.SeriesDescriptor{Namespace: "check", Name: fmt.Sprintf("metric_%05d", i), Host: "host-a", Tags: testCompositeTags(tags), Aggregate: observer.Aggregate(i % 4)}})
		}
	}
	rand.New(rand.NewSource(123)).Shuffle(len(anomalies), func(i, j int) { anomalies[i], anomalies[j] = anomalies[j], anomalies[i] })
	return anomalies
}

func TestCorrelationMemberKeysPreserveOutput(t *testing.T) {
	for _, n := range []int{0, 1, 16, 128} {
		anomalies := correlationFixture(n)
		// Preserve even legacy delimiter aliases and the first descriptor's
		// tag order when multiple input descriptors have the same key.
		if n > 0 {
			anomalies = append(anomalies,
				observer.Anomaly{Source: observer.SeriesDescriptor{Name: "alias", Tags: testCompositeTags([]string{"a,b"})}},
				observer.Anomaly{Source: observer.SeriesDescriptor{Name: "alias", Tags: testCompositeTags([]string{"a", "b"})}})
		}
		require.Equal(t, correlationMembersBefore(anomalies), sortedUniqueMembers(anomalies))
	}
}

// Previous member sorting, retained to verify identical output.
func correlationMembersBefore(anomalies []observer.Anomaly) []observer.SeriesDescriptor {
	seen := make(map[string]observer.SeriesDescriptor)
	for _, a := range anomalies {
		key := a.Source.Key()
		if _, ok := seen[key]; !ok {
			seen[key] = a.Source
		}
	}
	members := make([]observer.SeriesDescriptor, 0, len(seen))
	for _, sd := range seen {
		members = append(members, sd)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Key() < members[j].Key() })
	return members
}
