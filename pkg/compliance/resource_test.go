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

func intPtr(v int) *int {
	return &v
}

func TestResources(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected Resource
	}{
		{
			name: "file reporting owner",
			input: `
file:
  path: /etc/docker/daemon.json
  report:
  - attribute: owner
`,
			expected: Resource{
				File: &File{
					Path: `/etc/docker/daemon.json`,
					Report: Report{
						{
							Attribute: "owner",
						},
					},
				},
			},
		},
		{
			name: "file reporting permissions",
			input: `
file:
  path: /etc/docker/daemon.json
  report:
  - attribute: permissions
`,
			expected: Resource{
				File: &File{
					Path: `/etc/docker/daemon.json`,
					Report: Report{
						{
							Attribute: "permissions",
						},
					},
				},
			},
		},
		{
			name: "file with path from command reporting owner",
			input: `
file:
  pathFrom:
  - command: systemctl show -p FragmentPath docker.service
  report:
  - attribute: owner
`,
			expected: Resource{
				File: &File{
					PathFrom: ValueFrom{
						{
							Command: `systemctl show -p FragmentPath docker.service`,
						},
					},
					Report: Report{
						{
							Attribute: "owner",
						},
					},
				},
			},
		},
		{
			name: "file reporting jsonpath property",
			input: `
file:
  path: /etc/docker/daemon.json
  report:
  - jsonpath: tlsverify
`,
			expected: Resource{
				File: &File{
					Path: `/etc/docker/daemon.json`,
					Report: Report{
						{
							JSONPath: "tlsverify",
						},
					},
				},
			},
		},
		{
			name: "process reporting flag",
			input: `
process:
  name: dockerd
  report:
  - attribute: flag
    name: --tlsverify
    as: tlsverify
`,
			expected: Resource{
				Process: &Process{
					Name: "dockerd",
					Report: Report{
						{
							Attribute: "flag",
							Name:      "--tlsverify",
							As:        "tlsverify",
						},
					},
				},
			},
		},

		{
			name: "command reporting zero exit code as true",
			input: `
command:
  run: mountpoint -- "$(docker info -f '{{ .DockerRootDir }}')"
  filter:
  - include:
      exitCode: 0
  report:
  - as: docker_partition
    value: true
`,
			expected: Resource{
				Command: &Command{
					Run: `mountpoint -- "$(docker info -f '{{ .DockerRootDir }}')"`,
					Filter: []CommandFilter{
						{
							Include: &CommandCondition{
								ExitCode: intPtr(0),
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
			name: "audit with file path reporting as true",
			input: `
audit:
  path: /usr/bin/dockerd
  report:
  - as: audited
    value: true
`,
			expected: Resource{
				Audit: &Audit{
					Path: "/usr/bin/dockerd",
					Report: Report{
						{
							As:    "audited",
							Value: "true",
						},
					},
				},
			},
		},
		{
			name: "audit with file path from command reporting as true",
			input: `
audit:
  pathFrom:
  - command: systemctl show -p FragmentPath docker.socket
  report:
  - as: audited
    value: true
`,
			expected: Resource{
				Audit: &Audit{
					PathFrom: ValueFrom{
						{
							Command: `systemctl show -p FragmentPath docker.socket`,
						},
					},
					Report: Report{
						{
							As:    "audited",
							Value: "true",
						},
					},
				},
			},
		},
		{
			name: "group",
			input: `
group:
  name: docker
  report:
  - as: docker_group
`,
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

			name: "api with filter",
			input: `
api:
  kind: docker
  get: /images/{image_id}/json

  vars:
  - name: image_id
    enumerate:
      get: /images/json
      jsonpath: $.[Id]

  filter:
  - exclude:
      jsonpath: $.Config.Healthcheck
      exists: true

  report:
  - as: image_healthcheck_missing
    value: true

  - var: image_id
`,
			expected: Resource{
				API: &API{
					Kind: "docker",
					Get:  "/images/{image_id}/json",
					Vars: APIVars{
						{
							Name: "image_id",
							List: &APIVarValue{
								Get:      "/images/json",
								JSONPath: "$.[Id]",
							},
						},
					},
					Filter: []APIFilter{
						{
							Exclude: &APICondition{
								JSONPath: "$.Config.Healthcheck",
								Exists:   true,
							},
						},
					},
					Report: Report{
						{
							As:    "image_healthcheck_missing",
							Value: "true",
						},
						{
							Var: "image_id",
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
