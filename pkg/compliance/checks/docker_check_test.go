// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/docker/docker/api/types"

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
	b, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, obj)
}

func TestDockerImageCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "image",
			},
		},
		Condition: `docker.template("{{- $.Config.Healthcheck.Test -}}") != ""`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var images []types.ImageSummary
	assert.NoError(loadTestJSON("./testdata/docker/image-list.json", &images))
	client.On("ImageList", mockCtx, types.ImageListOptions{All: true}).Return(images, nil)

	imageIDMap := map[string]string{
		"sha256:09f3f4e9394f7620fb6f1025755c85dac07f7e7aa4fca4ba19e4a03590b63750": "./testdata/docker/image-09f3f4e9394f.json",
		"sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c": "./testdata/docker/image-f9b990972689.json",
		"sha256:89ec9da682137d6b18ab8244ca263b6771067f251562f884c7510c8f1e5ac910": "./testdata/docker/image-89ec9da68213.json",
	}

	for id, path := range imageIDMap {
		var image types.ImageInspect
		assert.NoError(loadTestJSON(path, &image))
		client.On("ImageInspectWithRaw", mockCtx, id).Return(image, nil, nil)
	}

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("DockerClient").Return(client)

	dockerCheck, err := newResourceCheck(env, "rule-id", resource)
	assert.NoError(err)

	reports := dockerCheck.check(env)

	expected := map[string]struct {
		Passed bool
		Tags   []string
	}{
		"sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c": {
			Passed: false,
			Tags:   []string{"redis:latest"},
		},
		"sha256:09f3f4e9394f7620fb6f1025755c85dac07f7e7aa4fca4ba19e4a03590b63750": {
			Passed: true,
			Tags:   []string{"nginx-healthcheck:latest"},
		},
		"sha256:89ec9da682137d6b18ab8244ca263b6771067f251562f884c7510c8f1e5ac910": {
			Passed: false,
			Tags:   []string{"nginx:alpine"},
		},
	}

	assert.Equal(len(reports), 3)

	for _, report := range reports {
		assert.Equal(expected[report.Data["image.id"].(string)].Passed, report.Passed, report.Data["image.id"])
		assert.Equal(expected[report.Data["image.id"].(string)].Tags, report.Data["image.tags"], report.Data["image.id"])
	}
}

func TestDockerNetworkCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "network",
			},
		},
		Condition: `docker.template("{{- index $.Options \"com.docker.network.bridge.default_bridge\" -}}") != "true" || docker.template("{{- index $.Options \"com.docker.network.bridge.enable_icc\" -}}") == "true"`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var networks []types.NetworkResource
	assert.NoError(loadTestJSON("./testdata/docker/network-list.json", &networks))
	client.On("NetworkList", mockCtx, types.NetworkListOptions{}).Return(networks, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("DockerClient").Return(client)

	dockerCheck, err := newResourceCheck(env, "rule-id", resource)
	assert.NoError(err)

	reports := dockerCheck.check(env)

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
			condition:    `docker.template("{{- $.AppArmorProfile -}}") not in ["", "unconfined"]`,
			expectPassed: false,
		},
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Docker: &compliance.DockerResource{
						Kind: "container",
					},
				},
				Condition: test.condition,
			}

			client := &mocks.DockerClient{}
			defer client.AssertExpectations(t)

			var containers []types.Container
			assert.NoError(loadTestJSON("./testdata/docker/container-list.json", &containers))
			client.On("ContainerList", mockCtx, types.ContainerListOptions{All: true}).Return(containers, nil)

			var container types.ContainerJSON
			assert.NoError(loadTestJSON("./testdata/docker/container-3c4bd9d35d42.json", &container))
			client.On("ContainerInspect", mockCtx, "3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87").Return(container, nil, nil)

			env := &mocks.Env{}
			defer env.AssertExpectations(t)

			env.On("DockerClient").Return(client)

			dockerCheck, err := newResourceCheck(env, "rule-id", resource)
			assert.NoError(err)

			reports := dockerCheck.check(env)

			assert.Equal(test.expectPassed, reports[0].Passed)
			assert.Equal("3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87", reports[0].Data["container.id"])
			assert.Equal("/sharp_cori", reports[0].Data["container.name"])
			assert.Equal("sha256:b4ceee5c3fa3cea2607d5e2bcc54d019be616e322979be8fc7a8d0d78b59a1f1", reports[0].Data["container.image"])
		})
	}
}

func TestDockerInfoCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "info",
			},
		},
		Condition: `docker.template("{{- $.RegistryConfig.InsecureRegistryCIDRs | join \",\" -}}") == ""`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var info types.Info
	assert.NoError(loadTestJSON("./testdata/docker/info.json", &info))
	client.On("Info", mockCtx).Return(info, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("DockerClient").Return(client)

	dockerCheck, err := newResourceCheck(env, "rule-id", resource)
	assert.NoError(err)

	reports := dockerCheck.check(env)

	assert.False(reports[0].Passed)
}

func TestDockerVersionCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		ResourceCommon: compliance.ResourceCommon{
			Docker: &compliance.DockerResource{
				Kind: "version",
			},
		},
		Condition: `docker.template("{{ range $.Components }}{{ if eq .Name \"Engine\" }}{{- .Details.Experimental -}}{{ end }}{{ end }}") == ""`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var version types.Version
	assert.NoError(loadTestJSON("./testdata/docker/version.json", &version))
	client.On("ServerVersion", mockCtx).Return(version, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)
	env.On("DockerClient").Return(client)

	dockerCheck, err := newResourceCheck(env, "rule-id", resource)
	assert.NoError(err)

	reports := dockerCheck.check(env)

	assert.False(reports[0].Passed)
	assert.Equal("19.03.6", reports[0].Data["docker.version"])
}
