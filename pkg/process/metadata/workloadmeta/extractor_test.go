// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

const (
	Pid1 = 1000
	Pid2 = 1001
)

func TestExtractor(t *testing.T) {
	wlmExtractor := NewWorkloadMetaExtractor()

	type testCase struct {
		name     string
		procs    map[int32]*procutil.Process
		expected map[int32]*languagedetection.Language
	}
	for _, tc := range []testCase{
		{
			name: "java & python",
			procs: map[int32]*procutil.Process{
				Pid1: {
					Pid:     Pid1,
					Cmdline: []string{"java", "TestClass"},
				},
				Pid2: {
					Pid:     Pid2,
					Cmdline: []string{"python", "main.py"},
				},
			},
			expected: map[int32]*languagedetection.Language{
				Pid1: {Name: languagedetection.Java},
				Pid2: {Name: languagedetection.Python},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wlmExtractor.Extract(tc.procs)
			for pid, lang := range tc.expected {
				proc := tc.procs[pid]
				if lang == nil {
					assert.Nil(t, proc.Language)
					continue
				}

				assert.Equal(t, lang.Name, proc.Language.Name)
			}
		})
	}
}
