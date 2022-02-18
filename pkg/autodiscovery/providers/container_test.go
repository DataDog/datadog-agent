// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// Only testing generateConfigs, lifecycle should be tested in end-to-end test

func TestGenerateConfigs_Container(t *testing.T) {
	configProvider := ContainerConfigProvider{
		containerCache: map[string]*workloadmeta.Container{
			"nolabels": {
				Runtime: workloadmeta.ContainerRuntimeContainerd,
			},
			"3b8efe0c50e8": {
				EntityMeta: workloadmeta.EntityMeta{
					Labels: map[string]string{
						"com.datadoghq.ad.check_names":  "[\"apache\",\"http_check\"]",
						"com.datadoghq.ad.init_configs": "[{}, {}]",
						"com.datadoghq.ad.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
		},
	}

	checks, err := configProvider.generateConfigs()
	assert.Nil(t, err)

	assert.Len(t, checks, 2)

	assert.Equal(t, []string{"docker://3b8efe0c50e8"}, checks[0].ADIdentifiers)
	assert.Equal(t, "{}", string(checks[0].InitConfig))
	assert.Equal(t, "container:docker://3b8efe0c50e8", checks[0].Source)
	assert.Len(t, checks[0].Instances, 1)
	assert.Equal(t, "{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}", string(checks[0].Instances[0]))
	assert.Equal(t, "apache", checks[0].Name)

	assert.Equal(t, []string{"docker://3b8efe0c50e8"}, checks[1].ADIdentifiers)
	assert.Equal(t, "{}", string(checks[1].InitConfig))
	assert.Equal(t, "container:docker://3b8efe0c50e8", checks[1].Source)
	assert.Len(t, checks[1].Instances, 1)
	assert.Equal(t, "{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}", string(checks[1].Instances[0]))
	assert.Equal(t, "http_check", checks[1].Name)
}
