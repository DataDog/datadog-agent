// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package misconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckProcMountHidePid(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name        string
		file        string
		groups      []int
		expectError string
	}{
		{
			name: "missing file",
			file: "./proc-mounts-not-here",
		},
		{
			name:        "hidepid with no groups",
			file:        "./tests/proc-mounts-hidepid-2",
			expectError: "hidepid=2 option detected in ./tests/proc-mounts-hidepid-2 - will prevent inspection of procfs",
		},
		{
			name:   "hidepid with no groups",
			file:   "./tests/proc-mounts-hidepid-2",
			groups: []int{234, 4242},
		},
		{
			name: "no hidepid",
			file: "./tests/proc-mounts-no-hidepid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkProcMountHidePid(test.file, test.groups)
			if test.expectError != "" {
				assert.EqualError(err, test.expectError)
			} else {
				assert.NoError(err)
			}
		})
	}
}
