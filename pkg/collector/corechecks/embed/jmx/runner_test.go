// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigureRunner(t *testing.T) {
	r := &runner{}

	r.initRunner()

	// Test for no instances in jmx check conf file
	initConfYaml := []byte("")
	instanceConfYaml := []byte("")

	err := r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)

	// Test basic jmx_url
	instanceConfYaml = []byte("jmx_url: foo\n")
	initConfYaml = []byte("tools_jar_path: some/path")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, r.jmxfetch.JavaToolsJarPath, "some/path")

	// Test options precedence
	r.jmxfetch.JavaToolsJarPath = ""
	instanceConfYaml = []byte("jmx_url: foo\n" +
		"tools_jar_path: some/other/path")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, r.jmxfetch.JavaToolsJarPath, "some/other/path")

	// Test jar paths
	initConfYaml = []byte("custom_jar_paths:\n" +
		"  - foo/\n" +
		"  - bar/\n")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(r.jmxfetch.JavaCustomJarPaths))
	assert.Contains(t, r.jmxfetch.JavaCustomJarPaths, "foo/")
	assert.Contains(t, r.jmxfetch.JavaCustomJarPaths, "bar/")

	// Test java options
	instanceConfYaml = []byte("java_options: -Xmx200 -Xms40\n")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, r.jmxfetch.JavaOptions, "-Xmx200 -Xms40")

	// Test java bin options
	instanceConfYaml = []byte("java_bin_path: /usr/local/java8/bin/java\n")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, r.jmxfetch.JavaBinPath, "/usr/local/java8/bin/java")

	// Once an option is set, it's set - further changes will not be enforced
	instanceConfYaml = []byte("java_bin_path: /opt/java/bin/java\n")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, r.jmxfetch.JavaBinPath, "/usr/local/java8/bin/java")

	// Test process regex with no tools - should fail
	r.jmxfetch.JavaToolsJarPath = ""
	instanceConfYaml = []byte("process_name_regex: regex\n")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.NotNil(t, err)

	instanceConfYaml = []byte("process_name_regex: regex\n" +
		"tools_jar_path: some/other/path")
	err = r.configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)

	// Configurations "pile" up
	assert.Equal(t, r.jmxfetch.JavaToolsJarPath, "some/other/path")
	assert.Equal(t, r.jmxfetch.JavaBinPath, "/usr/local/java8/bin/java")
	assert.Equal(t, r.jmxfetch.JavaOptions, "-Xmx200 -Xms40")
	assert.Equal(t, len(r.jmxfetch.JavaCustomJarPaths), 2)
	assert.Contains(t, r.jmxfetch.JavaCustomJarPaths, "foo/")
	assert.Contains(t, r.jmxfetch.JavaCustomJarPaths, "bar/")
}
