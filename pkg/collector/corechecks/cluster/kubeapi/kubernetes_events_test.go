// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"fmt"
	"testing"

	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	obj "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/urn"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/clustername"
	"k8s.io/apimachinery/pkg/types"
)

func createEvent(count int32, namespace, objname, objkind, objuid, component, hostname, reason string, message string, timestamp int64, alertType string) *v1.Event {
	return &v1.Event{
		InvolvedObject: v1.ObjectReference{
			Name:      objname,
			Kind:      objkind,
			UID:       types.UID(objuid),
			Namespace: namespace,
		},
		Count: count,
		Type:  alertType,
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
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", 709662600, "info")
	ev2 := createEvent(3, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Started", "Started container", 709662600, "info")
	ev3 := createEvent(1, "default", "localhost", "Node", "e63e74fa-f566-11e7-9749-0e4863e1cbf4", "kubelet", "machine-blue", "MissingClusterDNS", "MountVolume.SetUp succeeded", 709675200, "warning")
	ev4 := createEvent(29, "default", "localhost", "Node", "e63e74fa-f566-11e7-9749-0e4863e1cbf4", "kubelet", "machine-blue", "MissingClusterDNS", "MountVolume.SetUp succeeded", 709675200, "warning")
	// (As Object kinds are Pod and Node here, the event should take the remote hostname `machine-blue`)

	kubeApiEventsCheck := &EventsCheck{
		instance: &EventsConfig{
			FilteredEventType: []string{"ignored"},
		},
		CommonCheck: CommonCheck{
			CheckBase:             core.NewCheckBase(kubernetesAPIEventsCheckName),
			KubeAPIServerHostname: "hostname",
		},
		mapperFactory: func(d apiserver.OpenShiftDetector, clusterName string) *kubernetesEventMapper {
			return &kubernetesEventMapper{
				urn:         urn.NewURNBuilder(urn.Kubernetes, clusterName),
				clusterName: clusterName,
				sourceType:  string(urn.Kubernetes),
			}
		},
	}
	// Several new events, testing aggregation
	// Not testing full match of the event message as the order of the actions in the summary isn't guaranteed

	newKubeEventsBundle := []*v1.Event{
		ev1,
		ev2,
	}
	mocked := mocksender.NewMockSender(kubeApiEventsCheck.ID())
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))

	_ = kubeApiEventsCheck.processEvents(mocked, newKubeEventsBundle, false)

	// We are only expecting one bundle event.
	// We need to check that the countByAction concatenated string contains the source events.
	// As the order is not guaranteed we want to use contains.
	res1 := (mocked.Calls[0].Arguments.Get(0)).(metrics.Event)
	assert.Contains(t, res1.Title, "Scheduled - dca-789976f5d7-2ljx6 Pod")
	assert.Equal(t, "Activities", res1.EventContext.Category)
	mocked.AssertNumberOfCalls(t, "Event", 2)
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
		Tags:           []string{"kube_namespace:default", "source_component:kubelet"},
		SourceTypeName: "kubernetes",
		Ts:             709675200,
		Host:           "machine-blue",
		EventType:      "MissingClusterDNS",
		EventContext: &metrics.EventContext{
			Source:   "kubernetes",
			Category: "Alerts",
			ElementIdentifiers: []string{
				fmt.Sprintf("urn:kubernetes:/%s:node/localhost", clustername.GetClusterName()),
			},
		},
	}
	mocked = mocksender.NewMockSender(kubeApiEventsCheck.ID())
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))

	_ = kubeApiEventsCheck.processEvents(mocked, modifiedKubeEventsBundle, true)

	mocked.AssertEvent(t, modifiedNewDatadogEvents, 0)
	mocked.AssertExpectations(t)

	// Test the hostname change when a cluster name is set
	var testClusterName = "Laika"
	mockConfig := config.Mock()
	mockConfig.Set("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read
	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.Set("cluster_name", nil)
	defer clustername.ResetClusterName()

	modifiedNewDatadogEventsWithClusterName := metrics.Event{
		Title:          "Events from the machine-blue Node",
		Text:           "%%% \n30 **MissingClusterDNS**: MountVolume.SetUp succeeded\n \n _Events emitted by the kubelet seen at " + time.Unix(709675200, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"kube_namespace:default", "source_component:kubelet"},
		SourceTypeName: "kubernetes",
		Ts:             709675200,
		Host:           "machine-blue-" + testClusterName,
		EventType:      "MissingClusterDNS",
		EventContext: &metrics.EventContext{
			Source:   "kubernetes",
			Category: "Alerts",
			ElementIdentifiers: []string{
				fmt.Sprintf("urn:kubernetes:/%s:node/localhost", clustername.GetClusterName()),
			},
		},
	}

	mocked = mocksender.NewMockSender(kubeApiEventsCheck.ID())
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))

	_ = kubeApiEventsCheck.processEvents(mocked, modifiedKubeEventsBundle, true)

	mocked.AssertEvent(t, modifiedNewDatadogEventsWithClusterName, 0)
	mocked.AssertExpectations(t)
}

func TestProcessEvent(t *testing.T) {
	// We want to check if the format of 1 New event creates a DD event accordingly.
	// We also want to check that filtered and empty events aren't submitted
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "ReplicaSet", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", 709662600, "info")
	// (Object kind was changed from Pod to ReplicaSet to test the choice of hostname: it should take here the local hostname below `hostname`)

	kubeApiEventsCheck := &EventsCheck{
		instance: &EventsConfig{
			FilteredEventType: []string{"ignored"},
		},
		CommonCheck: CommonCheck{
			CheckBase:             core.NewCheckBase(kubernetesAPIEventsCheckName),
			KubeAPIServerHostname: "hostname",
		},
		mapperFactory: func(d apiserver.OpenShiftDetector, clusterName string) *kubernetesEventMapper {
			return &kubernetesEventMapper{
				urn:         urn.NewURNBuilder(urn.Kubernetes, clusterName),
				clusterName: clusterName,
				sourceType:  string(urn.Kubernetes),
			}
		},
	}
	mocked := mocksender.NewMockSender(kubeApiEventsCheck.ID())

	newKubeEventBundle := []*v1.Event{
		ev1,
	}
	// 1 Scheduled:
	newDatadogEvent := metrics.Event{
		Title:          "Events from the dca-789976f5d7-2ljx6 ReplicaSet",
		Text:           "%%% \n2 **Scheduled**: Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54\n \n _New events emitted by the default-scheduler seen at " + time.Unix(709662600000, 0).String() + "_ \n\n %%%",
		Priority:       "normal",
		Tags:           []string{"source_component:default-scheduler", "kube_namespace:default"},
		SourceTypeName: "kubernetes",
		Ts:             709662600,
		Host:           "",
		EventType:      "Scheduled",
		EventContext: &metrics.EventContext{
			Source:   "kubernetes",
			Category: "Activities",
			ElementIdentifiers: []string{
				fmt.Sprintf("urn:kubernetes:/%s:default:replicaset/dca-789976f5d7-2ljx6", clustername.GetClusterName()),
			},
		},
	}
	mocked.On("Event", mock.AnythingOfType("metrics.Event"))
	_ = kubeApiEventsCheck.processEvents(mocked, newKubeEventBundle, false)
	mocked.AssertEvent(t, newDatadogEvent, 0)
	mocked.AssertExpectations(t)

	// No events
	empty := []*v1.Event{}
	mocked = mocksender.NewMockSender(kubeApiEventsCheck.ID())
	_ = kubeApiEventsCheck.processEvents(mocked, empty, false)
	mocked.AssertNotCalled(t, "Event")
	mocked.AssertExpectations(t)

	// Ignored Event
	ev5 := createEvent(1, "default", "machine-blue", "Node", "529fe848-e132-11e7-bad4-0e4863e1cbf4", "kubelet", "machine-blue", "ignored", "", 709675200, "info")
	filteredKubeEventsBundle := []*v1.Event{
		ev5,
	}
	mocked = mocksender.NewMockSender(kubeApiEventsCheck.ID())
	_ = kubeApiEventsCheck.processEvents(mocked, filteredKubeEventsBundle, false)
	mocked.AssertNotCalled(t, "Event")
	mocked.AssertExpectations(t)
}
