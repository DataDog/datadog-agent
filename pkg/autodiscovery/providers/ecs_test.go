// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/stretchr/testify/assert"
)

func TestParseECSContainers(t *testing.T) {
	labels := map[string]string{
		"com.datadoghq.ad.check_names":  "[\"nginx\", \"http_check\"]",
		"com.datadoghq.ad.init_configs": "[{}, {}]",
		"com.datadoghq.ad.instances":    "[{\"nginx_status_url\": \"http://%%host%%\"}, {\"url\": \"http://%%host%%/healthz\"}]",
	}
	c := ecs.Container{
		DockerID: "deadbeef",
		Image:    "test",
		Labels:   labels,
	}
	tpls, err := parseECSContainers([]ecs.Container{c})
	assert.Nil(t, err)
	assert.Len(t, tpls, 2)
	assert.Equal(t, []string{"docker://deadbeef"}, tpls[0].ADIdentifiers)
	assert.Equal(t, "nginx", string(tpls[0].Name))
	assert.Equal(t, "{}", string(tpls[0].InitConfig))
	assert.Equal(t, "ecs:docker://deadbeef", string(tpls[0].Source))
	assert.Len(t, tpls[0].Instances, 1)
	assert.Equal(t, "{\"nginx_status_url\":\"http://%%host%%\"}", string(tpls[0].Instances[0]))

	assert.Equal(t, []string{"docker://deadbeef"}, tpls[1].ADIdentifiers)
	assert.Equal(t, "http_check", string(tpls[1].Name))
	assert.Equal(t, "{}", string(tpls[1].InitConfig))
	assert.Equal(t, "ecs:docker://deadbeef", string(tpls[1].Source))
	assert.Len(t, tpls[1].Instances, 1)
	assert.Equal(t, "{\"url\":\"http://%%host%%/healthz\"}", string(tpls[1].Instances[0]))

}
