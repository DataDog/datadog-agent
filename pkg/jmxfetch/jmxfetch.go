// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package jmxfetch

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	api "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	log "github.com/cihub/seelog"
)

const (
	jmxJarName                        = "jmxfetch-0.18.2-jar-with-dependencies.jar"
	jmxMainClass                      = "org.datadog.jmxfetch.App"
	defaultJmxCommand                 = "collect"
	defaultJvmMaxMemoryAllocation     = " -Xmx200m"
	defaultJvmInitialMemoryAllocation = " -Xms50m"
	defaultJavaBinPath                = "java"
	defaultLogLevel                   = "info"
)

var (
	jmxLogLevelMap = map[string]string{
		"trace":    "TRACE",
		"debug":    "DEBUG",
		"info":     "INFO",
		"warn":     "WARN",
		"warning":  "WARN",
		"error":    "ERROR",
		"err":      "ERROR",
		"critical": "FATAL",
	}
)

// JMXFetch represent a jmxfetch instance.
type JMXFetch struct {
	JavaBinPath        string
	JavaOptions        string
	JavaToolsJarPath   string
	JavaCustomJarPaths []string
	LogLevel           string
	JmxExitFile        string
	Command            string
	ReportOnConsole    bool
	ConfDirectory      string
	Checks             []string
	defaultJmxCommand  string
	cmd                *exec.Cmd
	exitFilePath       string
}

// New returns a new instance of JMXFetch with default values.
func New() *JMXFetch {
	return &JMXFetch{
		JavaBinPath:        defaultJavaBinPath,
		JavaCustomJarPaths: []string{},
		LogLevel:           defaultLogLevel,
		Command:            defaultJmxCommand,
		ReportOnConsole:    false,
		Checks:             []string{},
	}
}

// Run starts the JMXFetch process
func (j *JMXFetch) Run() error {
	here, _ := executable.Folder()
	classpath := filepath.Join(common.GetDistPath(), "jmx", jmxJarName)
	if j.JavaToolsJarPath != "" {
		classpath = fmt.Sprintf("%s:%s", j.JavaToolsJarPath, classpath)
	}

	globalCustomJars := config.Datadog.GetStringSlice("jmx_custom_jars")
	if len(globalCustomJars) > 0 {
		classpath = fmt.Sprintf("%s:%s", strings.Join(globalCustomJars, ":"), classpath)
	}

	if len(j.JavaCustomJarPaths) > 0 {
		classpath = fmt.Sprintf("%s:%s", strings.Join(j.JavaCustomJarPaths, ":"), classpath)
	}
	bindHost := config.Datadog.GetString("bind_host")
	if bindHost == "" || bindHost == "0.0.0.0" {
		bindHost = "localhost"
	}

	reporter := fmt.Sprintf("statsd:%s:%s", bindHost, config.Datadog.GetString("dogstatsd_port"))
	if j.ReportOnConsole {
		reporter = "console"
	}

	//TODO : support auto discovery

	subprocessArgs := []string{}

	// Specify a maximum memory allocation pool for the JVM
	javaOptions := j.JavaOptions
	if !strings.Contains(javaOptions, "Xmx") && !strings.Contains(javaOptions, "XX:MaxHeapSize") {
		javaOptions += defaultJvmMaxMemoryAllocation
	}
	// Specify the initial memory allocation pool for the JVM
	if !strings.Contains(javaOptions, "Xms") && !strings.Contains(javaOptions, "XX:InitialHeapSize") {
		javaOptions += defaultJvmInitialMemoryAllocation
	}

	subprocessArgs = append(subprocessArgs, strings.Fields(javaOptions)...)

	jmxLogLevel, ok := jmxLogLevelMap[strings.ToLower(j.LogLevel)]
	if !ok {
		jmxLogLevel = "INFO"
	}
	// checks are now enabled via IPC on JMXFetch
	subprocessArgs = append(subprocessArgs,
		"-classpath", classpath,
		jmxMainClass,
		"--ipc_host", config.Datadog.GetString("cmd_host"),
		"--ipc_port", fmt.Sprintf("%v", config.Datadog.GetInt("cmd_port")),
		"--check_period", fmt.Sprintf("%v", int(check.DefaultCheckInterval/time.Millisecond)), // Period of the main loop of jmxfetch in ms
		"--log_level", jmxLogLevel,
		"--reporter", reporter, // Reporter to use
	)

	if j.ConfDirectory != "" {
		subprocessArgs = append(subprocessArgs, "--check")
		subprocessArgs = append(subprocessArgs, j.Checks...)
		subprocessArgs = append(subprocessArgs, "--conf_directory", j.ConfDirectory)
	}

	subprocessArgs = append(subprocessArgs, j.Command)

	if j.JmxExitFile != "" {
		j.exitFilePath = filepath.Join(here, "dist", "jmx", j.JmxExitFile) // FIXME : At some point we should have a `run` folder
		// Signal handlers are not supported on Windows:
		// use a file to trigger JMXFetch exit instead
		subprocessArgs = append(subprocessArgs, "--exit_file_location", j.exitFilePath)
	}

	j.cmd = exec.Command(j.JavaBinPath, subprocessArgs...)

	// set environment + token
	j.cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("SESSION_TOKEN=%s", api.GetAuthToken()),
	)

	// remove the exit file trigger (windows)
	if j.JmxExitFile != "" {
		os.Remove(j.exitFilePath)
	}

	// forward the standard output to the Agent logger
	stdout, err := j.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			log.Info(in.Text())
		}
	}()

	// forward the standard error to the Agent logger
	stderr, err := j.cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			log.Error(in.Text())
		}
	}()

	log.Debugf("Args: %v", subprocessArgs)

	return j.cmd.Start()
}

// Kill kills the JMXFetch process
func (j *JMXFetch) Kill() error {
	if j.JmxExitFile == "" {
		// Unix
		err := j.cmd.Process.Signal(os.Kill)
		if err != nil {
			return err
		}
	} else {
		// Windows
		if err := ioutil.WriteFile(j.exitFilePath, nil, 0644); err != nil {
			return err
		}
	}
	return nil
}

// Wait waits for the end of the JMXFetch process and returns the error code
func (j *JMXFetch) Wait() error {
	return j.cmd.Wait()
}
