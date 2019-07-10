// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Only testing parseDockerLabels, lifecycle should be tested in end-to-end test

func TestParseDockerLabels(t *testing.T) {

	containers := map[string]map[string]string{
		"nolabels": {},
		"3b8efe0c50e8": {
			"com.datadoghq.ad.check_names":  "[\"apache\",\"http_check\"]",
			"com.datadoghq.ad.init_configs": "[{}, {}]",
			"com.datadoghq.ad.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
		},
	}

	checks, err := parseDockerLabels(containers)
	assert.Nil(t, err)

	assert.Len(t, checks, 2)

	assert.Equal(t, []string{"container_id://3b8efe0c50e8"}, checks[0].ADIdentifiers)
	assert.Equal(t, "{}", string(checks[0].InitConfig))
	assert.Len(t, checks[0].Instances, 1)
	assert.Equal(t, "{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}", string(checks[0].Instances[0]))
	assert.Equal(t, "apache", checks[0].Name)

	assert.Equal(t, []string{"container_id://3b8efe0c50e8"}, checks[1].ADIdentifiers)
	assert.Equal(t, "{}", string(checks[1].InitConfig))
	assert.Len(t, checks[1].Instances, 1)
	assert.Equal(t, "{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}", string(checks[1].Instances[0]))
	assert.Equal(t, "http_check", checks[1].Name)
}
