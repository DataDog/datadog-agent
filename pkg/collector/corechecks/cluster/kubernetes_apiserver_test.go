// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/ericchiang/k8s/api/v1"
	obj "github.com/ericchiang/k8s/apis/meta/v1"
	"testing"
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
	unhealthyComp := &v1.ComponentCondition{Type: toStr("Healthy"), Status: toStr("False")}

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
				Metadata: &obj.ObjectMeta{
					Name: toStr("Consul"),
				},
			},
		},
	}

	unhealthy := &v1.ComponentStatusList{
		Items: []*v1.ComponentStatus{
			{
				Conditions: []*v1.ComponentCondition{
					unhealthyComp,
				},
				Metadata: &obj.ObjectMeta{
					Name: toStr("ETCD"),
				},
			},
		},
	}

	kubeASCheck := &KubeASCheck{
		lastWarnings: []error{},
		instance: &KubeASConfig{
			Tags: []string{"test"},
		},
		KubeAPIServerHostname: "hostname",
	}

	mock := mocksender.NewMockSender(kubeASCheck.ID())
	mock.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "hostname", []string{"test", "component:Zookeeper"}, "")
	mock.On("Commit").Return()
	kubeASCheck.parseComponentStatus(mock, expected)

	mock.AssertNumberOfCalls(t, "Commit", 1)

	mock.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "hostname", []string{"test", "component:Consul"}, "The Component's condition type isn't supported")
	mock.On("Commit").Return()
	kubeASCheck.parseComponentStatus(mock, unExpected)
	mock.AssertNumberOfCalls(t, "Commit", 2)

	mock.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "hostname", []string{"test", "component:ETCD"}, "")
	mock.On("Commit").Return()
	kubeASCheck.parseComponentStatus(mock, unhealthy)
	mock.AssertNumberOfCalls(t, "Commit", 3)

	mock.AssertExpectations(t)
}
