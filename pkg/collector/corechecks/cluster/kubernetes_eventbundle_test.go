// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"

	cache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestFormatEvent(t *testing.T) {
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", "Normal", 709662600)
	ev2 := createEvent(3, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Started", "Started container", "Normal", 709662600)

	eventList := []*v1.Event{
		ev1,
		ev2,
	}
	b := &kubernetesEventBundle{
		events:        eventList,
		objUID:        types.UID("some_id"),
		component:     "Pod",
		countByAction: make(map[string]int),
	}

	expectedOutput := metrics.Event{
		Title:          fmt.Sprintf("Events from the %s", b.readableKey),
		Priority:       metrics.EventPriorityNormal,
		Host:           "",
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(b.lastTimestamp),
		Tags:           []string{fmt.Sprintf("source_component:%s", b.component), fmt.Sprintf("kubernetes_kind:%s", b.kind), fmt.Sprintf("name:%s", b.name)},
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", b.objUID),
	}
	expectedOutput.Text = "%%% \n" + fmt.Sprintf("%s \n _Events emitted by the %s seen at %s since %s_ \n", formatStringIntMap(b.countByAction), b.component, time.Unix(int64(b.lastTimestamp), 0), time.Unix(int64(b.timeStamp), 0)) + "\n %%%"

	providerIDCache := cache.New(defaultCacheExpire, defaultCachePurge)
	output, err := b.formatEvents("", providerIDCache)

	assert.Nil(t, err, "not nil")
	assert.Equal(t, expectedOutput, output)
}

func TestFormatEventWithNodename(t *testing.T) {
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", "Normal", 709662600)
	ev2 := createEvent(3, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Started", "Started container", "Normal", 709662600)

	eventList := []*v1.Event{
		ev1,
		ev2,
	}

	clusterName := "test_cluster"
	nodename := "test_nodename"
	providerID := "test_provider_ID"

	b := &kubernetesEventBundle{
		events:        eventList,
		objUID:        types.UID("some_id"),
		component:     "Pod",
		countByAction: make(map[string]int),
		nodename:      nodename,
	}

	expectedOutput := metrics.Event{
		Title:          fmt.Sprintf("Events from the %s", b.readableKey),
		Priority:       metrics.EventPriorityNormal,
		Host:           nodename + "-" + clusterName,
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(b.lastTimestamp),
		Tags:           []string{fmt.Sprintf("source_component:%s", b.component), fmt.Sprintf("kubernetes_kind:%s", b.kind), fmt.Sprintf("name:%s", b.name), fmt.Sprintf("host_provider_id:%s", providerID)},
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", b.objUID),
	}
	expectedOutput.Text = "%%% \n" + fmt.Sprintf("%s \n _Events emitted by the %s seen at %s since %s_ \n", formatStringIntMap(b.countByAction), b.component, time.Unix(int64(b.lastTimestamp), 0), time.Unix(int64(b.timeStamp), 0)) + "\n %%%"

	providerIDCache := cache.New(defaultCacheExpire, defaultCachePurge)
	providerIDCache.Set(nodename, providerID, cache.NoExpiration)
	output, err := b.formatEvents(clusterName, providerIDCache)

	assert.Nil(t, err, "not nil")
	assert.Equal(t, expectedOutput, output)
}

func Test_getDDAlertType(t *testing.T) {
	tests := []struct {
		name    string
		k8sType string
		want    metrics.EventAlertType
	}{
		{
			name:    "normal",
			k8sType: "Normal",
			want:    metrics.EventAlertTypeInfo,
		},
		{
			name:    "warning",
			k8sType: "Warning",
			want:    metrics.EventAlertTypeWarning,
		},
		{
			name:    "unknown",
			k8sType: "Unknown",
			want:    metrics.EventAlertTypeInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getDDAlertType(tt.k8sType); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getDDAlertType() = %v, want %v", got, tt.want)
			}
		})
	}
}
