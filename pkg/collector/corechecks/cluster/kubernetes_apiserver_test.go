// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"testing"

	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/api/core/v1"
	obj "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"k8s.io/apimachinery/pkg/types"
)

func TestParseComponentStatus(t *testing.T) {
	// We only test one Component Condition as only the Healthy Condition is supported.
	// We check if the OK, Critical and Unknown Service Checks are returned accordingly to the condition and the status given.

	expectedComp := v1.ComponentCondition{Type: "Healthy", Status: "True"}
	unExpectedComp := v1.ComponentCondition{Type: "Not Supported", Status: "True"}
	unHealthyComp := v1.ComponentCondition{Type: "Healthy", Status: "False"}
	unExpectedStatus := v1.ComponentCondition{Type: "Healthy", Status: "Other"}

	expected := &v1.ComponentStatusList{
		Items: []v1.ComponentStatus{
			{
				Conditions: []v1.ComponentCondition{
					expectedComp,
				},
				ObjectMeta: obj.ObjectMeta{
					Name: "Zookeeper",
				},
			},
		},
	}

	unExpected := &v1.ComponentStatusList{
		Items: []v1.ComponentStatus{
			{
				Conditions: []v1.ComponentCondition{
					unExpectedComp,
				},
				ObjectMeta: obj.ObjectMeta{
					Name: "",
				},
			},
		},
	}

	unHealthy := &v1.ComponentStatusList{
		Items: []v1.ComponentStatus{
			{
				Conditions: []v1.ComponentCondition{
					unHealthyComp,
				},
				ObjectMeta: obj.ObjectMeta{
					Name: "ETCD",
				},
			},
		},
	}
	unknown := &v1.ComponentStatusList{
		Items: []v1.ComponentStatus{
			{
				Conditions: []v1.ComponentCondition{
					unExpectedStatus,
				},
				ObjectMeta: obj.ObjectMeta{
					Name: "DCA",
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

	mocked := mocksender.NewMockSender(kubeASCheck.ID())
	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "hostname", []string{"test", "component:Zookeeper"}, "")
	kubeASCheck.parseComponentStatus(mocked, expected)

	mocked.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "hostname", []string{"test", "component:Zookeeper"}, "")

	err := kubeASCheck.parseComponentStatus(mocked, unExpected)
	assert.EqualError(t, err, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "hostname", []string{"test", "component:ETCD"}, "")
	kubeASCheck.parseComponentStatus(mocked, unHealthy)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 2)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "hostname", []string{"test", "component:ETCD"}, "")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "hostname", []string{"test", "component:DCA"}, "")
	kubeASCheck.parseComponentStatus(mocked, unknown)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 3)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "hostname", []string{"test", "component:DCA"}, "")

	empty_resp := kubeASCheck.parseComponentStatus(mocked, empty)
	assert.Nil(t, empty_resp, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.AssertExpectations(t)
}

func createEvent(count int32, namespace, objname, objkind, objuid, component, hostname, reason string, message string, timestamp int64) *v1.Event {
	return &v1.Event{
		InvolvedObject: v1.ObjectReference{
			Name:      objname,
			Kind:      objkind,
			UID:       types.UID(objuid),
			Namespace: namespace,
		},
		Count: count,
		Source: v1.EventSource{
			Component: component,
			Host:      hostname,
		},
		Reason: reason,
		FirstTimestamp: obj.Time{
			Time: time.Unix(timestamp, 0),
		},
		LastTimestamp: obj.Time{
			Time: time.Unix(timestamp, 0),
		},
		Message: message,
	}
}

func TestProcessBundledEvents(t *testing.T) {
	// We want to check if the format of several new events and several modified events creates DD events accordingly
	// We also want to check that a modified event with an existing key is aggregated (i.e. the key is already known)
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", 709662600)
	ev2 := createEvent(3, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Started", "Started container", 709662600)
	ev3 := createEvent(1, "default", "localhost", "Node", "e63e74fa-f566-11e7-9749-0e4863e1cbf4", "kubelet", "machine-blue", "MissingClusterDNS", "MountVolume.SetUp succeeded", 709662600)
	ev4 := createEvent(29, "default", "localhost", "Node", "e63e74fa-f566-11e7-9749-0e4863e1cbf4", "kubelet", "machine-blue", "MissingClusterDNS", "MountVolume.SetUp succeeded", 709675200)
	// (As Object kinds are Pod and Node here, the event should take the remote hostname `machine-blue`)

	kubeASCheck := &KubeASCheck{
		instance: &KubeASConfig{
			Tags:              []string{"test"},
			FilteredEventType: []string{"ignored"},
		},
		CheckBase:             core.NewCheckBase(kubernetesAPIServerCheckName),
		KubeAPIServerHostname: "hostname",
	}
	// Several new events, testing aggregation
	// Not testing full match of the event message as the order of the actions in the summary isn't guaranteed

	newKubeEventsBundle := []*v1.Event{
		ev1,
		ev2,
	}
	mocked := mocksender.NewMockSender(kubeASCheck.ID())
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))

	kubeASCheck.processEvents(mocked, newKubeEventsBundle, false)

	// We are only expecting one bundle event.
	// We need to check that the countByAction concatenated string contains the source events.
	// As the order is not guaranteed we want to use contains.
	res := (mocked.Calls[0].Arguments.Get(0)).(metrics.Event).Text
	assert.Contains(t, res, "2 **Scheduled**")
	assert.Contains(t, res, "3 **Started**")
	mocked.AssertNumberOfCalls(t, "Event", 1)
	mocked.AssertExpectations(t)

	// Several modified events, timestamp is the latest, event submitted has the correct key and count.
	modifiedKubeEventsBundle := []*v1.Event{
		ev3,
		ev4,
	}
	modifiedNewDatadogEvents := metrics.Event{
		Title:          "Events from the machine-blue Node",
		Text:           "%%% \n30 **MissingClusterDNS**: MountVolume.SetUp succeeded\n \n _Events emitted by the kubelet seen at " + time.Unix(709675200, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"test", "namespace:default", "source_component:kubelet"},
		AggregationKey: "kubernetes_apiserver:e63e74fa-f566-11e7-9749-0e4863e1cbf4",
		SourceTypeName: "kubernetes",
		Ts:             709675200,
		Host:           "machine-blue",
		EventType:      "kubernetes_apiserver",
	}
	mocked = mocksender.NewMockSender(kubeASCheck.ID())
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))

	kubeASCheck.processEvents(mocked, modifiedKubeEventsBundle, true)

	mocked.AssertEvent(t, modifiedNewDatadogEvents, 0)
	mocked.AssertExpectations(t)

	// Test the hostname change when a cluster name is set
	var testClusterName = "Laika"
	mockConfig := config.NewMock()
	mockConfig.Set("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read
	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.Set("cluster_name", nil)
	defer clustername.ResetClusterName()

	modifiedNewDatadogEventsWithClusterName := metrics.Event{
		Title:          "Events from the machine-blue Node",
		Text:           "%%% \n30 **MissingClusterDNS**: MountVolume.SetUp succeeded\n \n _Events emitted by the kubelet seen at " + time.Unix(709675200, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"test", "namespace:default", "source_component:kubelet"},
		AggregationKey: "kubernetes_apiserver:e63e74fa-f566-11e7-9749-0e4863e1cbf4",
		SourceTypeName: "kubernetes",
		Ts:             709675200,
		Host:           "machine-blue-" + testClusterName,
		EventType:      "kubernetes_apiserver",
	}

	mocked = mocksender.NewMockSender(kubeASCheck.ID())
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))

	kubeASCheck.processEvents(mocked, modifiedKubeEventsBundle, true)

	mocked.AssertEvent(t, modifiedNewDatadogEventsWithClusterName, 0)
	mocked.AssertExpectations(t)
}

func TestProcessEvent(t *testing.T) {
	// We want to check if the format of 1 New event creates a DD event accordingly.
	// We also want to check that filtered and empty events aren't submitted
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "ReplicaSet", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", 709662600)
	// (Object kind was changed from Pod to ReplicaSet to test the choice of hostname: it should take here the local hostname below `hostname`)

	kubeASCheck := &KubeASCheck{
		instance: &KubeASConfig{
			Tags:              []string{"test"},
			FilteredEventType: []string{"ignored"},
		},
		CheckBase:             core.NewCheckBase(kubernetesAPIServerCheckName),
		KubeAPIServerHostname: "hostname",
	}
	mocked := mocksender.NewMockSender(kubeASCheck.ID())

	newKubeEventBundle := []*v1.Event{
		ev1,
	}
	// 1 Scheduled:
	newDatadogEvent := metrics.Event{
		Title:          "Events from the dca-789976f5d7-2ljx6 ReplicaSet",
		Text:           "%%% \n2 **Scheduled**: Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54\n \n _New events emitted by the default-scheduler seen at " + time.Unix(709662600000, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"test", "source_component:default-scheduler", "namespace:default"},
		AggregationKey: "kubernetes_apiserver:e6417a7f-f566-11e7-9749-0e4863e1cbf4",
		SourceTypeName: "kubernetes",
		Ts:             709662600,
		Host:           "",
		EventType:      "kubernetes_apiserver",
	}
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))
	kubeASCheck.processEvents(mocked, newKubeEventBundle, false)
	mocked.AssertEvent(t, newDatadogEvent, 0)
	mocked.AssertExpectations(t)

	// No events
	empty := []*v1.Event{}
	mocked = mocksender.NewMockSender(kubeASCheck.ID())
	kubeASCheck.processEvents(mocked, empty, false)
	mocked.AssertNotCalled(t, "Event")
	mocked.AssertExpectations(t)

	// Ignored Event
	ev5 := createEvent(1, "default", "machine-blue", "Node", "529fe848-e132-11e7-bad4-0e4863e1cbf4", "kubelet", "machine-blue", "ignored", "", 709675200)
	filteredKubeEventsBundle := []*v1.Event{
		ev5,
	}
	mocked = mocksender.NewMockSender(kubeASCheck.ID())
	kubeASCheck.processEvents(mocked, filteredKubeEventsBundle, false)
	mocked.AssertNotCalled(t, "Event")
	mocked.AssertExpectations(t)
}
