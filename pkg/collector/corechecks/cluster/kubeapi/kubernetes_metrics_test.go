// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	obj "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
)

func TestParseComponentStatus(t *testing.T) {
	// We only test one Component Condition as only the Healthy Condition is supported.
	// We check if the OK, Critical and Unknown Service Checks are returned accordingly to the condition and the status given.

	expectedComp := v1.ComponentCondition{Type: "Healthy", Status: "True", Message: "imok"}
	unExpectedComp := v1.ComponentCondition{Type: "Not Supported", Status: "True", Message: ""}
	unHealthyComp := v1.ComponentCondition{Type: "Healthy", Status: "False", Error: "Connection closed"}
	unExpectedStatus := v1.ComponentCondition{Type: "Healthy", Status: "Other", Message: ""}

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

	kubeAPIMetricsCheck := NewKubernetesAPIMetricsCheck(core.NewCheckBase(kubernetesAPIMetricsCheckName), &MetricsConfig{})

	mocked := mocksender.NewMockSender(kubeAPIMetricsCheck.ID())
	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "", []string{"component:Zookeeper"}, "imok")
	_ = kubeAPIMetricsCheck.parseComponentStatus(mocked, expected)

	mocked.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "", []string{"component:Zookeeper"}, "imok")

	err := kubeAPIMetricsCheck.parseComponentStatus(mocked, unExpected)
	assert.EqualError(t, err, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "", []string{"component:ETCD"}, "Connection closed")
	_ = kubeAPIMetricsCheck.parseComponentStatus(mocked, unHealthy)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 2)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "", []string{"component:ETCD"}, "Connection closed")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "", []string{"component:DCA"}, "")
	_ = kubeAPIMetricsCheck.parseComponentStatus(mocked, unknown)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 3)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "", []string{"component:DCA"}, "")

	emptyResp := kubeAPIMetricsCheck.parseComponentStatus(mocked, empty)
	assert.Nil(t, emptyResp, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.AssertExpectations(t)
}
