// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestReset(t *testing.T) {
	tc := newTelemetryCache()
	tc.totalCount = 1337
	tc.unknownMetricsCount = 1337
	tc.metricsCountByResource = map[string]int{"foo": 1337}
	tc.reset()
	assert.Equal(t, 0, tc.totalCount)
	assert.Equal(t, 0, tc.unknownMetricsCount)
	assert.Len(t, tc.metricsCountByResource, 0)
}

func TestTotal(t *testing.T) {
	tc := newTelemetryCache()
	tc.incTotal(1337)
	tc.incTotal(1337)
	assert.Equal(t, 1337+1337, tc.getTotal())
}

func TestUnknown(t *testing.T) {
	tc := newTelemetryCache()
	tc.incUnknown()
	tc.incUnknown()
	assert.Equal(t, 2, tc.getUnknown())
}

func TestCountByResource(t *testing.T) {
	tc := newTelemetryCache()
	tc.incResource("foo", 1)
	tc.incResource("bar", 1)
	tc.incResource("foo", 1)
	assert.Equal(t, 2, tc.getResourcesCount()["foo"])
	assert.Equal(t, 1, tc.getResourcesCount()["bar"])
	assert.Len(t, tc.metricsCountByResource, 2)
}

// TestDoesNotUpdateInstanceTags exists to make sure that `sendTelemetry` does not update `instanceTags` by mistake in some edge cases
// First, if the array has enough capacity to append an element, no allocation is made. So at the initial state we have
// Tags: len(2), capacity(4), ["tag1:1", "tag1:1"]
// Then KSM calls append(tags, "resource_name:foo") which induces
// Tags: len(2), capacity(4), ["tag1:1", "tag1:1", "resource_name:foo"] /!\ the array is not updated
// Then, KSM uses the raw sender that calls pkg/util/sort_uniq.go(SortUniqInPlace) making Tags become
// Tags: len(2), capacity(4), ["tag1:1", "resource_name:foo", "tag1:1"]
// and here resource_name is added by mistake
func TestDoesNotUpdateInstanceTags(t *testing.T) {
	k := &KSMCheck{
		telemetry: newTelemetryCache(),
		instance: &KSMConfig{
			Tags: []string{
				"tag1:value1", "tag2:value2", "cluster_name:ali-cluster", "kube_cluster_name:ali-cluster",
			},
			Telemetry: true,
		},
	}

	// Apppend the same tag twice so one will be removed by SortUniqInPlace
	k.instance.Tags = append(k.instance.Tags, "kube_cluster_name:ali-cluster")
	// k.instance.Tags = append(k.instance.Tags, "tag1:1")

	y := k.instance.Tags

	y = util.SortUniqInPlace(y)

	// append the tag and sort uniq in place
	r := append(k.instance.Tags, "resource_name:foo")
	r = util.SortUniqInPlace(r)
	panic(fmt.Sprintf("%v %d %d", k.instance.Tags, len(k.instance.Tags), cap(k.instance.Tags)))
	assert.ElementsMatch(t, k.instance.Tags, []string{"tag:1"})
}
