// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/constants"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

var (
	mockCtx = mock.Anything
)

func loadTestJSON(path string, obj interface{}) error {
	jsonFile, err := os.Open(path)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(jsonFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, obj)
}

func dockerTestRule(resource compliance.RegoInput, kind, module string) *compliance.RegoRule {
	return &compliance.RegoRule{
		RuleCommon: compliance.RuleCommon{
			ID: "rule-id",
		},
		Inputs: []compliance.RegoInput{
			resource,
			{
				ResourceCommon: compliance.ResourceCommon{
					Constants: &compliance.ConstantsResource{
						Values: map[string]interface{}{
							"resource_type": kind,
						},
					},
				},
			},
		},
		Imports: []string{
			"../../rego/rego_helpers/helpers.rego",
		},
		Module: module,
	}
}

func TestDockerImageCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.RegoInput{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "image",
			},
		},
		TagName: "images",
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var images []types.ImageSummary
	assert.NoError(loadTestJSON("./testdata/image-list.json", &images))
	client.On("ImageList", mockCtx, types.ImageListOptions{All: true}).Return(images, nil)

	imageIDMap := map[string]string{
		"sha256:09f3f4e9394f7620fb6f1025755c85dac07f7e7aa4fca4ba19e4a03590b63750": "./testdata/image-09f3f4e9394f.json",
		"sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c": "./testdata/image-f9b990972689.json",
		"sha256:89ec9da682137d6b18ab8244ca263b6771067f251562f884c7510c8f1e5ac910": "./testdata/image-89ec9da68213.json",
	}

	for id, path := range imageIDMap {
		var image types.ImageInspect
		assert.NoError(loadTestJSON(path, &image))
		client.On("ImageInspectWithRaw", mockCtx, id).Return(image, nil, nil)
	}

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("DockerClient").Return(client)
	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()

	module := `package datadog

import data.datadog as dd
import data.helpers as h

findings[f] {
	image := input.images[_]
	f := dd.passed_finding(
			h.resource_type,
			h.docker_image_resource_id(image),
			h.docker_image_data(image),
	)
}`
	regoRule := dockerTestRule(resource, "docker_image", module)

	dockerCheck := rego.NewCheck(regoRule)
	err := dockerCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
	assert.NoError(err)

	reports := dockerCheck.Check(env)

	expected := map[string]struct {
		Passed bool
		Tags   []interface{}
	}{
		"sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c": {
			Passed: true,
			Tags:   []interface{}{"redis:latest"},
		},
		"sha256:09f3f4e9394f7620fb6f1025755c85dac07f7e7aa4fca4ba19e4a03590b63750": {
			Passed: true,
			Tags:   []interface{}{"nginx-healthcheck:latest"},
		},
		"sha256:89ec9da682137d6b18ab8244ca263b6771067f251562f884c7510c8f1e5ac910": {
			Passed: true,
			Tags:   []interface{}{"nginx:alpine"},
		},
	}

	assert.Equal(len(expected), len(reports))

	for _, report := range reports {
		expectedTags, expectedState := expected[report.Data["image.id"].(string)].Tags, expected[report.Data["image.id"].(string)].Passed
		assert.Equal(expectedState, report.Passed, report.Data["image.id"])
		assert.Equal(expectedTags, report.Data["image.tags"], report.Data["image.id"])
	}
}

func TestDockerNetworkCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.RegoInput{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "network",
			},
		},
		TagName: "networks",
		// Condition: `docker.template("{{- index $.Options \"com.docker.network.bridge.default_bridge\" -}}") != "true" || docker.template("{{- index $.Options \"com.docker.network.bridge.enable_icc\" -}}") == "true"`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var networks []types.NetworkResource
	assert.NoError(loadTestJSON("./testdata/network-list.json", &networks))
	client.On("NetworkList", mockCtx, types.NetworkListOptions{}).Return(networks, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("DockerClient").Return(client)
	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()

	module := `package datadog

import data.datadog as dd
import data.helpers as h

findings[f] {
	network := input.networks[_]
	f := dd.passed_finding(
			h.resource_type,
			h.docker_network_resource_id(network),
			h.docker_network_data(network),
	)
}`
	regoRule := dockerTestRule(resource, "docker_network", module)

	dockerCheck := rego.NewCheck(regoRule)
	err := dockerCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
	assert.NoError(err)

	reports := dockerCheck.Check(env)

	assert.True(reports[0].Passed)
	assert.Equal("bridge", reports[0].Data["network.name"])
}

func TestDockerContainerCheck(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name         string
		condition    string
		expectPassed bool
	}{
		{
			name:         "apparmor profile check 5.1",
			condition:    "container.inspect.AppArmorProfile != \"\"\ncontainer.inspect.AppArmorProfile != \"unconfined\"",
			expectPassed: false,
		},
		/*
			{
				name:         "selinux check 5.2",
				condition:    `docker.template("{{- has \"selinux\" $.HostConfig.SecurityOpt -}}") == "true"`,
				expectPassed: false,
			},
			{
				name:         "capadd check 5.3",
				condition:    `docker.template("{{ range $.HostConfig.CapAdd }}{{ if regexMatch \"AUDIT_WRITE|CHOWN|DAC_OVERRIDE|FOWNER|FSETID|KILL|MKNOD|NET_BIND_SERVICE|NET_RAW|SETFCAP|SETGID|SETPCAP|SETUID|SYS_CHROOT\" . }}failed{{ end }}{{ end }}") == ""`,
				expectPassed: false,
			},
			{
				name:         "privileged mode container 5.4",
				condition:    `docker.template("{{- $.HostConfig.Privileged -}}") != "true"`,
				expectPassed: false,
			},
			{
				name:         "host mounts check 5.5",
				condition:    `docker.template("{{ range $.Mounts }}{{ if has .Source (list \"/\" \"/boot\" \"/dev\" \"/etc\" \"/lib\" \"/proc\" \"/sys\" \"/usr\") }}{{ .Source }}{{ end }}{{ end }}") == ""`,
				expectPassed: false,
			},
			{
				name:         "privileged ports 5.7",
				condition:    `docker.template("{{ range $k, $_ := $.NetworkSettings.Ports }}{{ with $p := (regexReplaceAllLiteral \"/.*\" ($k | toString) \"\") | atoi }}{{ if lt $p 1024}}failed{{ end }}{{ end }}{{ end }}") == ""`,
				expectPassed: false,
			},
			{
				name:         "network mode check 5.9",
				condition:    `docker.template("{{- $.HostConfig.NetworkMode -}}") != "host"`,
				expectPassed: false,
			},
			{
				name:         "memory check 5.10",
				condition:    `docker.template("{{- $.HostConfig.Memory -}}") != "0"`,
				expectPassed: false,
			},
			{
				name:         "cpu shares check 5.11",
				condition:    `docker.template("{{- $.HostConfig.CpuShares -}}") not in ["0", "1024", ""]`,
				expectPassed: false,
			},
			{
				name:         "readonly rootfs check 5.12",
				condition:    `docker.template("{{- $.HostConfig.ReadonlyRootfs -}}") == "true"`,
				expectPassed: false,
			},
			{
				name:         "restart policy check 5.14",
				condition:    `docker.template("{{- $.HostConfig.RestartPolicy.Name -}}") == "on-failure" && docker.template("{{- eq $.HostConfig.RestartPolicy.MaximumRetryCount 5 -}}") == "true"`,
				expectPassed: false,
			},
			{
				name:         "pid mode check 5.15",
				condition:    `docker.template("{{- $.HostConfig.PidMode -}}") != "host"`,
				expectPassed: true,
			},
			{
				name:         "pids limit check 5.28",
				condition:    `docker.template("{{- $.HostConfig.PidsLimit -}}") not in ["", "<nil>", "0"]`,
				expectPassed: false,
			},
			{
				name:         "docker.sock check 5.31",
				condition:    `docker.template("{{ range $.Mounts }}{{ if eq .Source \"/var/run/docker.sock\" }}{{ .Source }}{{ end }}{{ end }}") == ""`,
				expectPassed: false,
			},
		*/
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Docker: &compliance.DockerResource{
						Kind: "container",
					},
				},
				TagName: "containers",
			}

			client := &mocks.DockerClient{}
			defer client.AssertExpectations(t)

			var containers []types.Container
			assert.NoError(loadTestJSON("./testdata/container-list.json", &containers))
			client.On("ContainerList", mockCtx, types.ContainerListOptions{}).Return(containers, nil)

			var container types.ContainerJSON
			assert.NoError(loadTestJSON("./testdata/container-3c4bd9d35d42.json", &container))
			client.On("ContainerInspect", mockCtx, "3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87").Return(container, nil, nil)

			env := &mocks.Env{}
			defer env.AssertExpectations(t)

			env.On("DockerClient").Return(client)
			env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
			env.On("DumpInputPath").Return("").Maybe()
			env.On("ShouldSkipRegoEval").Return(false).Maybe()
			env.On("Hostname").Return("test-host").Maybe()

			module := fmt.Sprintf(`package datadog
		
import data.datadog as dd
import data.helpers as h

valid_container(container) {
	%s
}

findings[f] {
	container := input.containers[_]
	valid_container(container)
	f := dd.passed_finding(
			h.resource_type,
			h.docker_container_resource_id(container),
			h.docker_container_data(container),
		)
}

findings[f] {
	container := input.containers[_]
	not valid_container(container)
	f := dd.failing_finding(
		h.resource_type,
		h.docker_container_resource_id(container),
		h.docker_container_data(container),
	)
}`, test.condition)

			regoRule := dockerTestRule(resource, "docker_container", module)

			dockerCheck := rego.NewCheck(regoRule)
			err := dockerCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
			assert.NoError(err)

			reports := dockerCheck.Check(env)

			t.Log(test.name)
			assert.Equal(test.expectPassed, reports[0].Passed)
			assert.Equal("3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87", reports[0].Data["container.id"])
			assert.Equal("/sharp_cori", reports[0].Data["container.name"])
			assert.Equal("redis:alpine", reports[0].Data["container.image"])
		})
	}
}

func TestDockerInfoCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.RegoInput{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "info",
			},
		},
		TagName: "infos",
		// Condition: `docker.template("{{- $.RegistryConfig.InsecureRegistryCIDRs | join \",\" -}}") == ""`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var info types.Info
	assert.NoError(loadTestJSON("./testdata/info.json", &info))
	client.On("Info", mockCtx).Return(info, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("DockerClient").Return(client)
	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()

	module := `package datadog

import data.datadog as dd
import data.helpers as h

valid_info(i) {
	i.inspect.RegistryConfig.InsecureRegistryCIDRs == null
}

valid_info(i) {
	count(i.inspect.RegistryConfig.InsecureRegistryCIDRs) == 0
}

valid_infos = [i | i := input.infos[_]; valid_info(i)]

findings[f] {
	count(valid_infos) == count(input.infos[_])
	f := dd.passed_finding(
			h.resource_type,
			h.resource_id,
			{},
	)
}

findings[f] {
	count(valid_infos) != count(input.infos[_])
		f := dd.failing_finding(
				h.resource_type,
				h.resource_id,
				{},
		)
}`

	regoRule := dockerTestRule(resource, "docker_daemon", module)

	dockerCheck := rego.NewCheck(regoRule)
	err := dockerCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
	assert.NoError(err)

	reports := dockerCheck.Check(env)

	assert.False(reports[0].Passed)
}

func TestDockerVersionCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.RegoInput{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "version",
			},
		},
		TagName: "version",
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var version types.Version
	assert.NoError(loadTestJSON("./testdata/version.json", &version))
	client.On("ServerVersion", mockCtx).Return(version, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("DockerClient").Return(client)
	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()

	module := `package datadog

import data.datadog as dd
import data.helpers as h

findings[f] {
	input.version.experimental == false
	f := dd.passed_finding(
			h.resource_type,
			h.resource_id,
			{ "docker.version": input.version.version },
	)
}

findings[f] {
	input.version.experimental == true
	f := dd.failing_finding(
			h.resource_type,
			h.resource_id,
			{ "docker.version": input.version.version },
	)
}`

	regoRule := dockerTestRule(resource, "docker_daemon", module)

	dockerCheck := rego.NewCheck(regoRule)
	err := dockerCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
	assert.NoError(err)

	reports := dockerCheck.Check(env)

	assert.True(reports[0].Passed)
	assert.Equal("19.03.6", reports[0].Data["docker.version"])
}
