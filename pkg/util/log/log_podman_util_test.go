// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ExtractPodmanRootDirFromDBPath(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"Rootless & BoltDB", "/data/containers_tomcat/storage/libpod/bolt_state.db", "/data/containers_tomcat"},
		{"Rootfull & BoltDB", "/var/lib/containers/storage/libpod/bolt_state.db", "/var/lib/containers"},
		{"Rootless & SQLite", "/home/ubuntu/.local/share/containers/storage/db.sql", "/home/ubuntu/.local/share/containers"},
		{"Rootfull & SQLite", "/var/lib/containers/storage/db.sql", "/var/lib/containers"},
		{"No matching suffix", "/foo/bar/baz", ""},
	}

	for _, testCase := range testCases {
		output := ExtractPodmanRootDirFromDBPath(testCase.input)
		assert.Equal(t, testCase.expected, output, fmt.Sprintf("%s: Expected %s but output is %s for input %s", testCase.name, testCase.expected, output, testCase.input))
	}

}
