// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	yaml "gopkg.in/yaml.v2"
)

const testResourceFile = `
file:
  path: /etc/docker/daemon.json
condition: file.owner == "root"
`

const testResourceProcess = `
process:
  name: dockerd
condition: process.flag("--tlsverify") != ""
`
const testResourceProcessWithFallback = `
process:
  name: dockerd
condition: process.flag("--tlsverify") != ""
fallback:
  condition: >-
    !process.hasFlag("--tlsverify")
  resource:
    file:
      path: /etc/docker/daemon.json
    condition: file.jq(".tlsverify") == "true"
`

const testResourceCommand = `
command:
  shell:
    run: mountpoint -- "$(docker info -f '{{ .DockerRootDir }}')"
condition: command.exitCode == 0
`

const testResourceAudit = `
audit:
  path: /usr/bin/dockerd
condition: audit.enabled
`

const testResourceGroup = `
group:
  name: docker
condition: >-
  "root" in group.users
`

const testResourceDockerImage = `
docker:
  kind: image
condition: docker.template("{{ $.Config.Healthcheck }}") != ""
`

func TestResources(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Resource
	}{
		{
			name:  "file",
			input: testResourceFile,
			expected: Resource{
				ResourceCommon: ResourceCommon{
					File: &File{
						Path: `/etc/docker/daemon.json`,
					},
				},
				Condition: `file.owner == "root"`,
			},
		},
		{
			name:  "process",
			input: testResourceProcess,
			expected: Resource{
				ResourceCommon: ResourceCommon{
					Process: &Process{
						Name: "dockerd",
					},
				},
				Condition: `process.flag("--tlsverify") != ""`,
			},
		},
		{
			name:  "process with fallback",
			input: testResourceProcessWithFallback,
			expected: Resource{
				ResourceCommon: ResourceCommon{
					Process: &Process{
						Name: "dockerd",
					},
				},
				Condition: `process.flag("--tlsverify") != ""`,
				Fallback: &Fallback{
					Condition: `!process.hasFlag("--tlsverify")`,
					Resource: Resource{
						ResourceCommon: ResourceCommon{
							File: &File{
								Path: `/etc/docker/daemon.json`,
							},
						},
						Condition: `file.jq(".tlsverify") == "true"`,
					},
				},
			},
		},
		{
			name:  "command",
			input: testResourceCommand,
			expected: Resource{
				ResourceCommon: ResourceCommon{
					Command: &Command{
						ShellCmd: &ShellCmd{
							Run: `mountpoint -- "$(docker info -f '{{ .DockerRootDir }}')"`,
						},
					},
				},
				Condition: `command.exitCode == 0`,
			},
		},
		{
			name:  "audit",
			input: testResourceAudit,
			expected: Resource{
				ResourceCommon: ResourceCommon{
					Audit: &Audit{
						Path: "/usr/bin/dockerd",
					},
				},
				Condition: `audit.enabled`,
			},
		},
		{
			name:  "group",
			input: testResourceGroup,
			expected: Resource{
				ResourceCommon: ResourceCommon{
					Group: &Group{
						Name: "docker",
					},
				},
				Condition: `"root" in group.users`,
			},
		},
		{

			name:  "docker image",
			input: testResourceDockerImage,
			expected: Resource{
				ResourceCommon: ResourceCommon{
					Docker: &DockerResource{
						Kind: "image",
					},
				},
				Condition: `docker.template("{{ $.Config.Healthcheck }}") != ""`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var r Resource
			err := yaml.Unmarshal([]byte(test.input), &r)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, r)
		})
	}
}
