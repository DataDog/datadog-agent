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
	kubeApiMetricsCheck := &MetricsCheck{
		CommonCheck: CommonCheck{
			CheckBase:             core.NewCheckBase(kubernetesAPIMetricsCheckName),
			KubeAPIServerHostname: "hostname",
		},
		instance: &MetricsConfig{},
	}

	mocked := mocksender.NewMockSender(kubeApiMetricsCheck.ID())
	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "hostname", []string{"component:Zookeeper"}, "")
	_ = kubeApiMetricsCheck.parseComponentStatus(mocked, expected)

	mocked.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckOK, "hostname", []string{"component:Zookeeper"}, "")

	err := kubeApiMetricsCheck.parseComponentStatus(mocked, unExpected)
	assert.EqualError(t, err, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "hostname", []string{"component:ETCD"}, "")
	_ = kubeApiMetricsCheck.parseComponentStatus(mocked, unHealthy)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 2)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckCritical, "hostname", []string{"component:ETCD"}, "")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "hostname", []string{"component:DCA"}, "")
	_ = kubeApiMetricsCheck.parseComponentStatus(mocked, unknown)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 3)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", metrics.ServiceCheckUnknown, "hostname", []string{"component:DCA"}, "")

	empty_resp := kubeApiMetricsCheck.parseComponentStatus(mocked, empty)
	assert.Nil(t, empty_resp, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.AssertExpectations(t)
}
