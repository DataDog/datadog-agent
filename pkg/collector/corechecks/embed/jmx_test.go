// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package embed

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/jmxfetch"

	"github.com/stretchr/testify/assert"
)

func TestReadJMXConf(t *testing.T) {
	check := &JMXCheck{
		runner: &jmxfetch.JMXFetch{},
	}

	// Test for no instances in jmx check conf file
	initConfYaml := []byte("")
	instanceConfYaml := []byte("")

	err := check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)

	// Test basic jmx_url
	instanceConfYaml = []byte("jmx_url: foo\n")
	initConfYaml = []byte(" tools_jar_path: some/path")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, check.runner.JavaToolsJarPath, "some/path")

	// Test options precedence
	check.runner.JavaToolsJarPath = ""
	instanceConfYaml = []byte("jmx_url: foo\n" +
		"tools_jar_path: some/other/path")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, check.runner.JavaToolsJarPath, "some/other/path")

	// Test jar paths
	initConfYaml = []byte("custom_jar_paths:\n" +
		"  - foo/\n" +
		"  - bar/\n")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, len(check.runner.JavaCustomJarPaths), 2)
	assert.Contains(t, check.runner.JavaCustomJarPaths, "foo/")
	assert.Contains(t, check.runner.JavaCustomJarPaths, "bar/")

	// Test java options
	instanceConfYaml = []byte("java_options: -Xmx200 -Xms40\n")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, check.runner.JavaOptions, "-Xmx200 -Xms40")

	// Test java bin options
	instanceConfYaml = []byte("java_bin_path: /usr/local/java8/bin/java\n")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, check.runner.JavaBinPath, "/usr/local/java8/bin/java")

	// Once an option is set, it's set - further changes will not be enforced
	instanceConfYaml = []byte("java_bin_path: /opt/java/bin/java\n")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, check.runner.JavaBinPath, "/usr/local/java8/bin/java")

	// Test process regex with no tools - should fail
	check.runner.JavaToolsJarPath = ""
	instanceConfYaml = []byte("process_name_regex: regex\n")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.EqualError(t, err, fmt.Sprintf("You must specify the path to tools.jar. %s", linkToDoc))

	instanceConfYaml = []byte("process_name_regex: regex\n" +
		"tools_jar_path: some/other/path")
	err = check.Configure(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.True(t, check.isAttachAPI)

	// Configurations "pile" up
	assert.Equal(t, check.runner.JavaToolsJarPath, "some/other/path")
	assert.Equal(t, check.runner.JavaBinPath, "/usr/local/java8/bin/java")
	assert.Equal(t, check.runner.JavaOptions, "-Xmx200 -Xms40")
	assert.Equal(t, len(check.runner.JavaCustomJarPaths), 2)
	assert.Contains(t, check.runner.JavaCustomJarPaths, "foo/")
	assert.Contains(t, check.runner.JavaCustomJarPaths, "bar/")
	assert.True(t, check.isAttachAPI)
}
