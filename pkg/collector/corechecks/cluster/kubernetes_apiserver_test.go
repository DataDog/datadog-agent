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
	"github.com/stretchr/testify/assert"
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

	kubeASCheck := &KubeASCheck{
		lastWarnings: []error{},
		instance: &KubeASConfig{
			Tags: []string{"test"},
		},
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
