package jmx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigureRunner(t *testing.T) {
	initRunner()

	// Test for no instances in jmx check conf file
	initConfYaml := []byte("")
	instanceConfYaml := []byte("")

	err := configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)

	// Test basic jmx_url
	instanceConfYaml = []byte("jmx_url: foo\n")
	initConfYaml = []byte("tools_jar_path: some/path")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, runner.JavaToolsJarPath, "some/path")

	// Test options precedence
	runner.JavaToolsJarPath = ""
	instanceConfYaml = []byte("jmx_url: foo\n" +
		"tools_jar_path: some/other/path")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, runner.JavaToolsJarPath, "some/other/path")

	// Test jar paths
	initConfYaml = []byte("custom_jar_paths:\n" +
		"  - foo/\n" +
		"  - bar/\n")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(runner.JavaCustomJarPaths))
	assert.Contains(t, runner.JavaCustomJarPaths, "foo/")
	assert.Contains(t, runner.JavaCustomJarPaths, "bar/")

	// Test java options
	instanceConfYaml = []byte("java_options: -Xmx200 -Xms40\n")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, runner.JavaOptions, "-Xmx200 -Xms40")

	// Test java bin options
	instanceConfYaml = []byte("java_bin_path: /usr/local/java8/bin/java\n")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, runner.JavaBinPath, "/usr/local/java8/bin/java")

	// Once an option is set, it's set - further changes will not be enforced
	instanceConfYaml = []byte("java_bin_path: /opt/java/bin/java\n")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)
	assert.Equal(t, runner.JavaBinPath, "/usr/local/java8/bin/java")

	// Test process regex with no tools - should fail
	runner.JavaToolsJarPath = ""
	instanceConfYaml = []byte("process_name_regex: regex\n")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.NotNil(t, err)

	instanceConfYaml = []byte("process_name_regex: regex\n" +
		"tools_jar_path: some/other/path")
	err = configureRunner(instanceConfYaml, initConfYaml)
	assert.Nil(t, err)

	// Configurations "pile" up
	assert.Equal(t, runner.JavaToolsJarPath, "some/other/path")
	assert.Equal(t, runner.JavaBinPath, "/usr/local/java8/bin/java")
	assert.Equal(t, runner.JavaOptions, "-Xmx200 -Xms40")
	assert.Equal(t, len(runner.JavaCustomJarPaths), 2)
	assert.Contains(t, runner.JavaCustomJarPaths, "foo/")
	assert.Contains(t, runner.JavaCustomJarPaths, "bar/")
}
