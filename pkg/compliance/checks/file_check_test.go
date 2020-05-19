// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
)

func TestFileCheck(t *testing.T) {
	tests := []struct {
		name       string
		file       *compliance.File
		expectedKV compliance.KV
	}{
		{
			name: "permissions",
			file: &compliance.File{
				Path: "./testdata/file/644.dat",
				Report: compliance.Report{
					{
						Property: "permissions",
						Kind:     "attribute",
					},
				},
			},
			expectedKV: compliance.KV{
				"permissions": "644",
			},
		},
		{
			name: "owner nobody:nobody",
			file: &compliance.File{
				Path: "./testdata/file/nobody-nobody.dat",
				Report: compliance.Report{
					{
						Property: "owner",
						Kind:     "attribute",
					},
				},
			},
			expectedKV: compliance.KV{
				"owner": "nobody:nobody",
			},
		},
		{
			name: "owner 2048:2048",
			file: &compliance.File{
				Path: "./testdata/file/2048-2048.dat",
				Report: compliance.Report{
					{
						Property: "owner",
						Kind:     "attribute",
					},
				},
			},
			expectedKV: compliance.KV{
				"owner": "2048:2048",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := &compliance.MockReporter{}
			fc := &fileCheck{
				baseCheck: newTestBaseCheck(reporter),
				File:      test.file,
			}
			reporter.On(
				"Report",
				newTestRuleEvent(
					nil,
					test.expectedKV,
				),
			).Once()

			err := fc.Run()
			assert.NoError(t, err)
		})
	}
}
