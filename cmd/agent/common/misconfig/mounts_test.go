// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

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
			name:        "missing file",
			file:        "./proc-mounts-not-here",
			expectError: "failed to open ./proc-mounts-not-here - proc fs inspection may not work: open ./proc-mounts-not-here: no such file or directory",
		},
		{
			name:        "hidepid without groups",
			file:        "./tests/proc-mounts-hidepid-2-groups",
			expectError: "hidepid=2 option detected in ./tests/proc-mounts-hidepid-2-groups - proc fs inspection may not work",
		},
		{
			name:   "hidepid and groups",
			file:   "./tests/proc-mounts-hidepid-2-groups",
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
