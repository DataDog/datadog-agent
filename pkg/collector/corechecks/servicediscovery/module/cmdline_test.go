// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
)

func TestTruncateCmdline(t *testing.T) {
	type testData struct {
		lang     language.Language
		original []string
		result   []string
	}

	plain := strings.Repeat("a", maxCommandLine*2)
	split := strings.Split(plain, "")
	classPath := strings.Join(split, ":")
	truncClassPath := strings.Join(append(split[:numPreserveClassPathEntries], "..."), ":")

	tests := []testData{
		{
			original: []string{},
			result:   nil,
		},
		{
			original: []string{"a", "b", "", "c", "d"},
			result:   []string{"a", "b", "c", "d"},
		},
		{
			original: []string{"x", plain[:maxCommandLine-1]},
			result:   []string{"x", plain[:maxCommandLine-1]},
		},
		{
			original: []string{plain[:maxCommandLine], "B"},
			result:   []string{plain[:maxCommandLine]},
		},
		{
			original: []string{plain[:maxCommandLine+1]},
			result:   []string{plain[:maxCommandLine]},
		},
		{
			original: []string{plain[:maxCommandLine-1], "", "B"},
			result:   []string{plain[:maxCommandLine-1], "B"},
		},
		{
			original: []string{plain[:maxCommandLine-1], "BCD"},
			result:   []string{plain[:maxCommandLine-1], "B"},
		},
		{
			lang: language.Go,
			original: []string{"foo",
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine]},
			result: []string{"foo",
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine-maxClassPath-1-len("foo")-len("-cp")]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine]},
			result: []string{"java",
				"-cp", truncClassPath,
				plain[:maxCommandLine-len(truncClassPath)-len("java")-len("-cp")]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine-maxClassPath-1-len("java")-len("-cp")]},
			result: []string{"java",
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine-maxClassPath-1-len("java")-len("-cp")]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"-cp", classPath[:maxClassPath+1],
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine]},
			result: []string{"java",
				"-cp", truncClassPath,
				"-cp", truncClassPath,
				plain[:maxCommandLine-len("java")-2*(len(truncClassPath)+len("-cp"))]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"-cp", classPath[:maxClassPath+1],
				"-jar", "foo.jar",
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine]},
			result: []string{"java",
				"-cp", truncClassPath,
				"-jar", "foo.jar",
				"-cp", classPath[:maxClassPath+1],
				plain[:maxCommandLine-len("java")-len(truncClassPath)-len("-cp")-len("-jar")-len("foo.jar")-len("-cp")-maxClassPath-1]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"-classpath", classPath[:maxClassPath+1],
				plain[:maxCommandLine]},
			result: []string{"java",
				"-classpath", truncClassPath,
				plain[:maxCommandLine-len(truncClassPath)-len("java")-len("-classpath")]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"--class-path", classPath[:maxClassPath+1],
				plain[:maxCommandLine]},
			result: []string{"java",
				"--class-path", truncClassPath,
				plain[:maxCommandLine-len(truncClassPath)-len("java")-len("--class-path")]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"--class-path=" + classPath[:maxClassPath+1],
				plain[:maxCommandLine]},
			result: []string{"java",
				"--class-path=" + truncClassPath,
				plain[:maxCommandLine-len(truncClassPath)-len("java")-len("--class-path=")]},
		},
		{
			lang:     language.Java,
			original: []string{"java", "-cp", classPath[:maxClassPath*2]},
			result:   []string{"java", "-cp", classPath[:maxClassPath*2]},
		},
		{
			lang:     language.Java,
			original: []string{"java", "-cp", ".", plain[:maxCommandLine]},
			result:   []string{"java", "-cp", ".", plain[:maxCommandLine-len("java")-len("-cp")-len(".")]},
		},
		{
			lang: language.Java,
			original: []string{"java",
				"-cp", "a:b:cdef",
				plain[:maxCommandLine]},
			result: []string{"java",
				"-cp", "a:b:cdef",
				plain[:maxCommandLine-len("java")-len("-cp")-len("a:b:cdef")]},
		},
		{
			lang:     language.Java,
			original: []string{"java", "-cp", plain},
			result:   []string{"java", "-cp", plain[:maxCommandLine-len("java")-len("-cp")]},
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.result, truncateCmdline(test.lang, test.original))
	}
}
