// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestFileCheck(t *testing.T) {
	tests := []struct {
		name     string
		file     *compliance.File
		setup    func(t *testing.T) *compliance.File
		validate func(t *testing.T, kv compliance.KV)
	}{
		{
			name: "permissions",
			setup: func(t *testing.T) *compliance.File {
				dir := os.TempDir()
				path := path.Join(dir, fmt.Sprintf("test-permissions-file-check-%d.dat", time.Now().Unix()))
				f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
				defer f.Close()
				assert.NoError(t, err)
				return &compliance.File{
					Path: path,
					Report: compliance.Report{
						{
							Property: "permissions",
							Kind:     "attribute",
						},
					},
				}
			},
			validate: func(t *testing.T, kv compliance.KV) {
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
			validate: func(t *testing.T, kv compliance.KV) {
				owner, ok := kv["owner"]
				assert.True(t, ok)
				parts := strings.SplitN(owner, ":", 2)
				assert.Equal(t, parts[0], "root")
				assert.Contains(t, []string{"root", "wheel"}, parts[1])
			},
		},
		{
			name: "jsonpath log-driver",
			file: &compliance.File{
				Path: "./testdata/file/daemon.json",
				Report: compliance.Report{
					{
						Property: "$['log-driver']",
						Kind:     "jsonpath",
						As:       "log_driver",
					},
				},
			},
			validate: func(t *testing.T, kv compliance.KV) {
				assert.Equal(t, compliance.KV{
					"log_driver": "json-file",
				}, kv)
			},
		},
		{
			name: "jsonpath experimental",
			file: &compliance.File{
				Path: "./testdata/file/daemon.json",
				Report: compliance.Report{
					{
						Property: "$.experimental",
						Kind:     "jsonpath",
						As:       "experimental",
					},
				},
			},
			validate: func(t *testing.T, kv compliance.KV) {
				assert.Equal(t, compliance.KV{
					"experimental": "false",
				}, kv)
			},
		},
		{
			name: "jsonpath ulimits",
			file: &compliance.File{
				Path: "./testdata/file/daemon.json",
				Report: compliance.Report{
					{
						Property: "$['default-ulimits'].nofile.Hard",
						Kind:     "jsonpath",
						As:       "nofile_hard",
					},
				},
			},
			validate: func(t *testing.T, kv compliance.KV) {
				assert.Equal(t, compliance.KV{
					"nofile_hard": "64000",
				}, kv)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := test.file
			if test.setup != nil {
				file = test.setup(t)
			}
			reporter := &compliance.MockReporter{}
			fc := &fileCheck{
				baseCheck: newTestBaseCheck(reporter),
				File:      file,
			}
			reporter.On(
				"Report",
				mock.AnythingOfType("*compliance.RuleEvent"),
			).Run(func(args mock.Arguments) {
				event := args.Get(0).(*compliance.RuleEvent)
				test.validate(t, event.Data)
			})

			err := fc.Run()
			assert.NoError(t, err)
		})
	}
}
