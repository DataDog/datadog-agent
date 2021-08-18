// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSuite(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		expectSuite *Suite
		expectError error
	}{
		{
			name: "supported version",
			file: "./testdata/cis-docker.yaml",
			expectSuite: &Suite{
				Meta: SuiteMeta{
					Schema: SuiteSchema{
						Version: "1.0",
					},
					Name:      "CIS Docker Generic",
					Framework: "cis-docker",
					Version:   "1.2.0",
					Source:    "./testdata/cis-docker.yaml",
				},
				Rules: []Rule{
					{
						RuleCommon: RuleCommon{
							ID:           "cis-docker-1",
							Scope:        RuleScopeList{DockerScope},
							HostSelector: `"foo" in node.labels`,
						},
						Resources: []Resource{
							{
								ResourceCommon: ResourceCommon{
									File: &File{
										Path: "/etc/docker/daemon.json",
									},
								},
								Condition: `file.permissions == 0644`,
							},
						},
					},
				},
			},
		},
		{
			name:        "unsupported version",
			file:        "./testdata/cis-docker-unsupported.yaml",
			expectError: ErrUnsupportedSchemaVersion,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := ParseSuite(test.file)
			assert.Equal(t, test.expectError, err)
			assert.Equal(t, test.expectSuite, actual)
		})
	}
}
