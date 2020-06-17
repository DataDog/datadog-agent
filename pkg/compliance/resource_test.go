// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	yaml "gopkg.in/yaml.v2"
)

const testResourceFileReportingOwner = `
file:
  path: /etc/docker/daemon.json
  report:
  - property: owner
    kind: attribute
`

const testResourceFileReportingPermissions = `
file:
  path: /etc/docker/daemon.json
  report:
  - property: permissions
    kind: attribute
`

const testResourceFilePathFromCommand = `
file:
  pathFrom:
  - command: systemctl show -p FragmentPath docker.service
  report:
  - property: owner
    kind: attribute
`

const testResourceFileReportingJSONPath = `
file:
  path: /etc/docker/daemon.json
  report:
  - property: tlsverify
    kind: jsonpath
`
const testResourceProcessReportingFlag = `
process:
  name: dockerd
  report:
  - property: --tlsverify
    kind: flag
    as: tlsverify
`

const testResourceCommand = `
command:
  shell:
    run: mountpoint -- "$(docker info -f '{{ .DockerRootDir }}')"
  filter:
  - include:
      exitCode: 0
  report:
  - as: docker_partition
    value: true
`

const testResourceAudit = `
audit:
  path: /usr/bin/dockerd
  report:
  - property: enabled
    kind: attribute
`

const testResourceAuditPathFromCommand = `
audit:
  pathFrom:
  - command: systemctl show -p FragmentPath docker.socket
  report:
  - property: enabled
    kind: attribute
`

const testResourceGroup = `
group:
  name: docker
  report:
  - as: docker_group
`

const testResourceDockerImageWithFilter = `
docker:
  kind: image

  filter:
  - exclude:
      property: "{{ $.Config.Healthcheck }}"
      kind: template
      op: exists

  report:
  - property: id
    as: image_id

  - as: image_healthcheck_missing
    value: true
`

func TestResources(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected Resource
	}{
		{
			name:  "file reporting owner",
			input: testResourceFileReportingOwner,
			expected: Resource{
				File: &File{
					Path: `/etc/docker/daemon.json`,
					Report: Report{
						{
							Property: "owner",
							Kind:     PropertyKindAttribute,
						},
					},
				},
			},
		},
		{
			name:  "file reporting permissions",
			input: testResourceFileReportingPermissions,
			expected: Resource{
				File: &File{
					Path: `/etc/docker/daemon.json`,
					Report: Report{
						{
							Property: "permissions",
							Kind:     PropertyKindAttribute,
						},
					},
				},
			},
		},
		{
			name:  "file with path from command reporting owner",
			input: testResourceFilePathFromCommand,
			expected: Resource{
				File: &File{
					PathFrom: ValueFrom{
						{
							Command: `systemctl show -p FragmentPath docker.service`,
						},
					},
					Report: Report{
						{
							Property: "owner",
							Kind:     PropertyKindAttribute,
						},
					},
				},
			},
		},
		{
			name:  "file reporting jsonpath property",
			input: testResourceFileReportingJSONPath,
			expected: Resource{
				File: &File{
					Path: `/etc/docker/daemon.json`,
					Report: Report{
						{
							Property: "tlsverify",
							Kind:     PropertyKindJSONPath,
						},
					},
				},
			},
		},
		{
			name:  "process reporting flag",
			input: testResourceProcessReportingFlag,
			expected: Resource{
				Process: &Process{
					Name: "dockerd",
					Report: Report{
						{
							Property: "--tlsverify",
							Kind:     PropertyKindFlag,
							As:       "tlsverify",
						},
					},
				},
			},
		},

		{
			name:  "command reporting zero exit code as true",
			input: testResourceCommand,
			expected: Resource{
				Command: &Command{
					ShellCmd: &ShellCmd{
						Run: `mountpoint -- "$(docker info -f '{{ .DockerRootDir }}')"`,
					},
					Filter: []CommandFilter{
						{
							Include: &CommandCondition{
								ExitCode: 0,
							},
						},
					},
					Report: Report{
						{
							As:    "docker_partition",
							Value: "true",
						},
					},
				},
			},
		},
		{
			name:  "audit with file path",
			input: testResourceAudit,
			expected: Resource{
				Audit: &Audit{
					Path: "/usr/bin/dockerd",
					Report: Report{
						{
							Property: "enabled",
							Kind:     "attribute",
						},
					},
				},
			},
		},
		{
			name:  "audit with file path from command",
			input: testResourceAuditPathFromCommand,
			expected: Resource{
				Audit: &Audit{
					PathFrom: ValueFrom{
						{
							Command: `systemctl show -p FragmentPath docker.socket`,
						},
					},
					Report: Report{
						{
							Property: "enabled",
							Kind:     "attribute",
						},
					},
				},
			},
		},
		{
			name:  "group",
			input: testResourceGroup,
			expected: Resource{
				Group: &Group{
					Name: "docker",
					Report: Report{
						{
							As: "docker_group",
						},
					},
				},
			},
		},
		{

			name:  "docker image with filter",
			input: testResourceDockerImageWithFilter,
			expected: Resource{
				Docker: &DockerResource{
					Kind: "image",

					Filter: []Filter{
						{
							Exclude: &Condition{
								Property:  "{{ $.Config.Healthcheck }}",
								Kind:      PropertyKindTemplate,
								Operation: OpExists,
							},
						},
					},
					Report: Report{
						{
							Property: "id",
							As:       "image_id",
						},
						{
							As:    "image_healthcheck_missing",
							Value: "true",
						},
					},
				},
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
