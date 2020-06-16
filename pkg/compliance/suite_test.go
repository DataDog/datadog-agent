// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSuite(t *testing.T) {
	expected := &Suite{
		Meta: SuiteMeta{
			Name:      "CIS Docker Generic",
			Framework: "cis-docker",
			Version:   "1.2.0",
		},
		Rules: []Rule{
			{
				ID: "cis-docker-1",
				Scope: Scope{
					Docker: true,
				},
				Resources: []Resource{
					{
						File: &File{
							Path: "/etc/docker/daemon.json",
							Report: []ReportedField{
								{
									Property: "permissions",
									Kind:     PropertyKindAttribute,
								},
							},
						},
					},
				},
			},
		},
	}

	actual, err := ParseSuite("./testdata/cis-docker.yaml")
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}
