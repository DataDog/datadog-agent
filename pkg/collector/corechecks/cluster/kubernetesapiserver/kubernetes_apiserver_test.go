// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	obj "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
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

	tagger := taggerimpl.SetupFakeTagger(t)

	// FIXME: use the factory instead
	kubeASCheck := NewKubeASCheck(core.NewCheckBase(CheckName), &KubeASConfig{}, tagger)

	mocked := mocksender.NewMockSender(kubeASCheck.ID())
	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", servicecheck.ServiceCheckOK, "", []string{"component:Zookeeper"}, "imok")
	kubeASCheck.parseComponentStatus(mocked, expected)

	mocked.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", servicecheck.ServiceCheckOK, "", []string{"component:Zookeeper"}, "imok")

	err := kubeASCheck.parseComponentStatus(mocked, unExpected)
	assert.EqualError(t, err, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", servicecheck.ServiceCheckCritical, "", []string{"component:ETCD"}, "Connection closed")
	kubeASCheck.parseComponentStatus(mocked, unHealthy)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 2)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", servicecheck.ServiceCheckCritical, "", []string{"component:ETCD"}, "Connection closed")

	mocked.On("ServiceCheck", "kube_apiserver_controlplane.up", servicecheck.ServiceCheckUnknown, "", []string{"component:DCA"}, "")
	kubeASCheck.parseComponentStatus(mocked, unknown)
	mocked.AssertNumberOfCalls(t, "ServiceCheck", 3)
	mocked.AssertServiceCheck(t, "kube_apiserver_controlplane.up", servicecheck.ServiceCheckUnknown, "", []string{"component:DCA"}, "")

	emptyResp := kubeASCheck.parseComponentStatus(mocked, empty)
	assert.Nil(t, emptyResp, "metadata structure has changed. Not collecting API Server's Components status")
	mocked.AssertNotCalled(t, "ServiceCheck", "kube_apiserver_controlplane.up")

	mocked.AssertExpectations(t)
}

func TestConvertFilter(t *testing.T) {
	for n, tc := range []struct {
		caseName string
		filters  []string
		output   string
	}{
		{
			caseName: "legacy support",
			filters:  []string{"OOM"},
			output:   "reason!=OOM",
		},
		{
			caseName: "exclude node and type",
			filters:  []string{"involvedObject.kind!=Node", "type==Normal"},
			output:   "involvedObject.kind!=Node,type==Normal",
		},
		{
			caseName: "legacy support and exclude HorizontalPodAutoscaler",
			filters:  []string{"involvedObject.kind!=HorizontalPodAutoscaler", "type!=Normal", "OOM"},
			output:   "involvedObject.kind!=HorizontalPodAutoscaler,type!=Normal,reason!=OOM",
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", n, tc.caseName), func(t *testing.T) {
			output := convertFilters(tc.filters)
			assert.Equal(t, tc.output, output)
		})
	}
}
