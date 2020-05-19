// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestFileCheck(t *testing.T) {
	tests := []struct {
		name string
		file *compliance.File
		onKV func(t *testing.T, kv compliance.KV)
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
			onKV: func(t *testing.T, kv compliance.KV) {
				assert.Equal(t, compliance.KV{
					"permissions": "644",
				}, kv)
			},
		},
		{
			name: "owner root",
			file: &compliance.File{
				Path: "/tmp",
				Report: compliance.Report{
					{
						Property: "owner",
						Kind:     "attribute",
					},
				},
			},
			onKV: func(t *testing.T, kv compliance.KV) {
				owner, ok := kv["owner"]
				assert.True(t, ok)
				parts := strings.SplitN(owner, ":", 2)
				assert.Equal(t, parts[0], "root")
				assert.Contains(t, []string{"root", "wheel"}, parts[1])
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
				mock.AnythingOfType("*compliance.RuleEvent"),
			).Run(func(args mock.Arguments) {
				event := args.Get(0).(*compliance.RuleEvent)
				test.onKV(t, event.Data)
			})

			err := fc.Run()
			assert.NoError(t, err)
		})
	}
}
