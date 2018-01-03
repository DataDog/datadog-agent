// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// Only testing parseKubeletPodlist, lifecycle should be tested in end-to-end test

func TestParseKubeletPodlist(t *testing.T) {
	podlist := []*kubelet.Pod{
		{
			Status: kubelet.Status{
				Containers: []kubelet.ContainerStatus{
					{
						Name: "testName",
						ID:   "testID",
					},
				},
			},
		},
		{
			Metadata: kubelet.PodMetadata{
				Annotations: map[string]string{
					"service-discovery.datadoghq.com/apache.check_names":  "[\"apache\",\"http_check\"]",
					"service-discovery.datadoghq.com/apache.init_configs": "[{},{}]",
					"service-discovery.datadoghq.com/apache.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
			},
			Status: kubelet.Status{
				Containers: []kubelet.ContainerStatus{
					{
						Name: "apache",
						ID:   "docker://3b8efe0c50e8",
					},
					{
						Name: "testName",
						ID:   "testID",
					},
				},
			},
		},
	}

	checks, err := parseKubeletPodlist(podlist)
	assert.Nil(t, err)

	assert.Len(t, checks, 2)

	assert.Equal(t, []string{"docker://3b8efe0c50e8"}, checks[0].ADIdentifiers)
	assert.Equal(t, "{}", string(checks[0].InitConfig))
	assert.Len(t, checks[0].Instances, 1)
	assert.Equal(t, "{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}", string(checks[0].Instances[0]))
	assert.Equal(t, "apache", checks[0].Name)

	assert.Equal(t, []string{"docker://3b8efe0c50e8"}, checks[1].ADIdentifiers)
	assert.Equal(t, "{}", string(checks[1].InitConfig))
	assert.Len(t, checks[1].Instances, 1)
	assert.Equal(t, "{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}", string(checks[1].Instances[0]))
	assert.Equal(t, "http_check", checks[1].Name)
}
