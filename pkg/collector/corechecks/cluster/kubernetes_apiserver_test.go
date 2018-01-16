// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"testing"

	"github.com/ericchiang/k8s/api/v1"
	obj "github.com/ericchiang/k8s/apis/meta/v1"
	"github.com/stretchr/testify/assert"

	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func toStr(str string) *string {
	ptrStr := str
	return &ptrStr
}

func TestParseComponentStatus(t *testing.T) {
	// We only test one Component Condition as only the Healthy Condition is supported.
	// We check if the OK, Critical and Unknown Service Checks are returned accordingly to the condition and the status given.

	expectedComp := &v1.ComponentCondition{Type: toStr("Healthy"), Status: toStr("True")}
	unExpectedComp := &v1.ComponentCondition{Type: toStr("Not Supported"), Status: toStr("True")}
	unHealthyComp := &v1.ComponentCondition{Type: toStr("Healthy"), Status: toStr("False")}
	unExpectedStatus := &v1.ComponentCondition{Type: toStr("Healthy"), Status: toStr("Other")}

	expected := &v1.ComponentStatusList{
		Items: []*v1.ComponentStatus{
			{
				Conditions: []*v1.ComponentCondition{
					expectedComp,
				},
				Metadata: &obj.ObjectMeta{
					Name: toStr("Zookeeper"),
				},
			},
		},
	}

	unExpected := &v1.ComponentStatusList{
		Items: []*v1.ComponentStatus{
			{
				Conditions: []*v1.ComponentCondition{
					unExpectedComp,
				},
				Metadata: nil,
			},
		},
	}

	unHealthy := &v1.ComponentStatusList{
		Items: []*v1.ComponentStatus{
			{
				Conditions: []*v1.ComponentCondition{
					unHealthyComp,
				},
				Metadata: &obj.ObjectMeta{
					Name: toStr("ETCD"),
				},
			},
		},
	}
	unknown := &v1.ComponentStatusList{
		Items: []*v1.ComponentStatus{
			{
				Conditions: []*v1.ComponentCondition{
					unExpectedStatus,
				},
				Metadata: &obj.ObjectMeta{
					Name: toStr("DCA"),
				},
			},
		},
	}
	empty := &v1.ComponentStatusList{
		Items: nil,
	}

	// FIXME: use the factory instead
	kubeASCheck := &KubeASCheck{
		instance: &KubeASConfig{
			Tags: []string{"test"},
		},
		CheckBase:             core.NewCheckBase(kubernetesAPIServerCheckName),
		KubeAPIServerHostname: "hostname",
	}

	mock := mocksender.NewMockSender(kubeASCheck.ID())
	mock.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "hostname", []string{"test", "component:Zookeeper"}, "")
	kubeASCheck.parseComponentStatus(mock, expected)

	mock.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mock.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "hostname", []string{"test", "component:Zookeeper"}, "")

	err := kubeASCheck.parseComponentStatus(mock, unExpected)
	assert.EqualError(t, err, "metadata structure has changed. Not collecting API Server's Components status")
	mock.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mock.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "hostname", []string{"test", "component:ETCD"}, "")
	kubeASCheck.parseComponentStatus(mock, unHealthy)
	mock.AssertNumberOfCalls(t, "ServiceCheck", 2)
	mock.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "hostname", []string{"test", "component:ETCD"}, "")

	mock.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "hostname", []string{"test", "component:DCA"}, "")
	kubeASCheck.parseComponentStatus(mock, unknown)
	mock.AssertNumberOfCalls(t, "ServiceCheck", 3)
	mock.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "hostname", []string{"test", "component:DCA"}, "")

	empty_resp := kubeASCheck.parseComponentStatus(mock, empty)
	assert.Nil(t, empty_resp, "metadata structure has changed. Not collecting API Server's Components status")
	mock.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mock.AssertExpectations(t)
}

func createEvent(count int32, objname, objkind, objuid, component, reason string, message string, timestamp int64) *v1.Event {
	return &v1.Event{
		Metadata: &obj.ObjectMeta{
			CreationTimestamp: &obj.Time{
				Seconds: &timestamp,
			},
		},
		InvolvedObject: &v1.ObjectReference{
			Name: &objname,
			Kind: &objkind,
			Uid:  &objuid,
		},
		Count: &count,
		Source: &v1.EventSource{
			Component: &component,
		},
		Reason: &reason,
		LastTimestamp: &obj.Time{
			Seconds: &timestamp,
		},
		Message: &message,
	}
}

func TestProcessEvents(t *testing.T) {
	// We want to check if the format of 1 New event, several new events and several modified events creates DD events accordingly
	// We also want to check that a modified event with an existing key is aggregated (i.e. the key is already known)

	ev1 := createEvent(2, "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", 709662600)
	ev2 := createEvent(3, "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "Started", "Started container", 709662600)
	ev3 := createEvent(1, "localhost", "Node", "e63e74fa-f566-11e7-9749-0e4863e1cbf4", "kubelet", "MissingClusterDNS", "MountVolume.SetUp succeeded", 709662600)
	ev4 := createEvent(29, "localhost", "Node", "e63e74fa-f566-11e7-9749-0e4863e1cbf4", "kubelet", "MissingClusterDNS", "MountVolume.SetUp succeeded", 709675200)

	kubeASCheck := &KubeASCheck{
		instance: &KubeASConfig{
			Tags:              []string{"test"},
			FilteredEventType: []string{"ignored"},
		},
		CheckBase:             core.NewCheckBase(kubernetesAPIServerCheckName),
		KubeAPIServerHostname: "hostname",
	}
	mock := mocksender.NewMockSender(kubeASCheck.ID())

	newKubeEventBundle := []*v1.Event{
		ev1,
	}
	// 1 Scheduled:
	newDatadogEvent := metrics.Event{
		Title:          "Events from the dca-789976f5d7-2ljx6 Pod",
		Text:           "%%% \n2 **Scheduled**: Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54\n \n _New events emitted by the default-scheduler seen at " + time.Unix(709662600, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"test"},
		AggregationKey: "kubernetes_apiserver:e6417a7f-f566-11e7-9749-0e4863e1cbf4",
		SourceTypeName: "kubernetes",
		Ts:             709662600,
		Host:           "hostname",
		EventType:      "kubernetes_apiserver",
	}
	mock.On("Event", newDatadogEvent).Times(1)
	kubeASCheck.processEvents(mock, newKubeEventBundle, false)
	mock.AssertEvent(t, newDatadogEvent, 0)
	mock.AssertExpectations(t)

	// Several new events, testing aggregation
	// Not testing full match of the event message as the order of the actions in the summary isn't guaranteed

	newKubeEventsBundle := []*v1.Event{
		ev1,
		ev2,
	}
	newDatadogEvents := metrics.Event{
		Title:          "Events from the dca-789976f5d7-2ljx6 Pod",
		Text:           "%%% \n2 **Scheduled**: Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54\n 3 **Started**: Started container\n \n _New events emitted by the default-scheduler seen at " + time.Unix(709662600, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"test"},
		AggregationKey: "kubernetes_apiserver:e6417a7f-f566-11e7-9749-0e4863e1cbf4",
		SourceTypeName: "kubernetes",
		Ts:             709662600,
		Host:           "hostname",
		EventType:      "kubernetes_apiserver",
	}

	mock = mocksender.NewMockSender(kubeASCheck.ID())
	mock.On("Event", newDatadogEvents).Times(1)

	kubeASCheck.processEvents(mock, newKubeEventsBundle, false)
	assert.Contains(t, newDatadogEvents.Text, "2 **Scheduled**")
	assert.Contains(t, newDatadogEvents.Text, "3 **Started**")
	mock.AssertNumberOfCalls(t, "Event", 1)
	mock.AssertExpectations(t)

	// Several modified events, timestamp is the latest, event submitted has the correct key and count.

	modifiedKubeEventsBundle := []*v1.Event{
		ev3,
		ev4,
	}
	modifiedNewDatadogEvents := metrics.Event{
		Title:          "Events from the localhost Node",
		Text:           "%%% \n30 **MissingClusterDNS**: MountVolume.SetUp succeeded\n \n _Events emitted by the kubelet seen at " + time.Unix(709675200, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"test"},
		AggregationKey: "kubernetes_apiserver:e63e74fa-f566-11e7-9749-0e4863e1cbf4",
		SourceTypeName: "kubernetes",
		Ts:             709675200,
		Host:           "hostname",
		EventType:      "kubernetes_apiserver",
	}
	mock = mocksender.NewMockSender(kubeASCheck.ID())
	mock.On("Event", modifiedNewDatadogEvents).Times(1)

	kubeASCheck.processEvents(mock, modifiedKubeEventsBundle, true)

	mock.AssertEvent(t, modifiedNewDatadogEvents, 0)
	mock.AssertExpectations(t)

	// No events
	empty := []*v1.Event{}
	mock = mocksender.NewMockSender(kubeASCheck.ID())
	kubeASCheck.processEvents(mock, empty, false)
	mock.AssertNotCalled(t, "Event")
	mock.AssertExpectations(t)

	// Ignored Event
	ev5 := createEvent(1, "localhost", "Node", "529fe848-e132-11e7-bad4-0e4863e1cbf4", "kubelet", "ignored", "", 709675200)
	filteredKubeEventsBundle := []*v1.Event{
		ev5,
	}
	mock = mocksender.NewMockSender(kubeASCheck.ID())
	kubeASCheck.processEvents(mock, filteredKubeEventsBundle, false)
	mock.AssertNotCalled(t, "Event")
	mock.AssertExpectations(t)
}
