// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build jmx

package jmxfetch

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	api "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	jmxJarName                        = "jmxfetch.jar"
	jmxMainClass                      = "org.datadog.jmxfetch.App"
	defaultJmxCommand                 = "collect"
	defaultJvmMaxMemoryAllocation     = " -Xmx200m"
	defaultJvmInitialMemoryAllocation = " -Xms50m"
	jvmCgroupMemoryAwareness          = " -XX:+UnlockExperimentalVMOptions -XX:+UseCGroupMemoryLimitForHeap"
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
	jvmCgroupMemoryIncompatOptions = []string{
		"Xmx",
		"XX:MaxHeapSize",
		"Xms",
		"XX:InitialHeapSize",
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
	Checks             []string
	IPCPort            int
	IPCHost            string
	defaultJmxCommand  string
	cmd                *exec.Cmd
	exitFilePath       string
	managed            bool
	shutdown           chan struct{}
	stopped            chan struct{}
}

func (j *JMXFetch) setDefaults() {
	if j.JavaBinPath == "" {
		j.JavaBinPath = defaultJavaBinPath
	}
	if j.JavaCustomJarPaths == nil {
		j.JavaCustomJarPaths = []string{}
	}
	if j.LogLevel == "" {
		j.LogLevel = defaultLogLevel
	}
	if j.Command == "" {
		j.Command = defaultJmxCommand
	}
	if j.Checks == nil {
		j.Checks = []string{}
	}
}

// Start starts the JMXFetch process
func (j *JMXFetch) Start(manage bool) error {
	j.setDefaults()

	here, _ := executable.Folder()
	classpath := filepath.Join(common.GetDistPath(), "jmx", jmxJarName)
	if j.JavaToolsJarPath != "" {
		classpath = fmt.Sprintf("%s%s%s", j.JavaToolsJarPath, string(os.PathListSeparator), classpath)
	}

	globalCustomJars := config.Datadog.GetStringSlice("jmx_custom_jars")
	if len(globalCustomJars) > 0 {
		classpath = fmt.Sprintf("%s%s%s", strings.Join(globalCustomJars, string(os.PathListSeparator)), string(os.PathListSeparator), classpath)
	}

	if len(j.JavaCustomJarPaths) > 0 {
		classpath = fmt.Sprintf("%s%s%s", strings.Join(j.JavaCustomJarPaths, string(os.PathListSeparator)), string(os.PathListSeparator), classpath)
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
	if config.Datadog.GetBool("jmx_use_cgroup_memory_limit") {
		passOption := true
		// This option is incompatible with the Xmx and Xms options, log a warning if there are found in the javaOptions
		for _, option := range jvmCgroupMemoryIncompatOptions {
			if strings.Contains(javaOptions, option) {
				log.Warnf("Java option %q is incompatible with cgroup_memory_limit, disabling cgroup mode", option)
				passOption = false
			}
		}
		if passOption {
			javaOptions += jvmCgroupMemoryAwareness
		}
	} else {
		// Specify a maximum memory allocation pool for the JVM
		if !strings.Contains(javaOptions, "Xmx") && !strings.Contains(javaOptions, "XX:MaxHeapSize") {
			javaOptions += defaultJvmMaxMemoryAllocation
		}
		// Specify the initial memory allocation pool for the JVM
		if !strings.Contains(javaOptions, "Xms") && !strings.Contains(javaOptions, "XX:InitialHeapSize") {
			javaOptions += defaultJvmInitialMemoryAllocation
		}
	}

	subprocessArgs = append(subprocessArgs, strings.Fields(javaOptions)...)

	jmxLogLevel, ok := jmxLogLevelMap[strings.ToLower(j.LogLevel)]
	if !ok {
		jmxLogLevel = "INFO"
	}

	ipcHost := config.Datadog.GetString("cmd_host")
	ipcPort := config.Datadog.GetInt("cmd_port")
	if j.IPCHost != "" {
		ipcHost = j.IPCHost
	}
	if j.IPCPort != 0 {
		ipcPort = j.IPCPort
	}

	// checks are now enabled via IPC on JMXFetch
	subprocessArgs = append(subprocessArgs,
		"-classpath", classpath,
		jmxMainClass,
		"--ipc_host", ipcHost,
		"--ipc_port", fmt.Sprintf("%v", ipcPort),
		"--check_period", fmt.Sprintf("%v", config.Datadog.GetInt("jmx_check_period")), // Period of the main loop of jmxfetch in ms
		"--thread_pool_size", fmt.Sprintf("%v", config.Datadog.GetInt("jmx_thread_pool_size")), // Size for the JMXFetch thread pool
		"--collection_timeout", fmt.Sprintf("%v", config.Datadog.GetInt("jmx_collection_timeout")), // Timeout for metric collection in seconds
		"--reconnection_timeout", fmt.Sprintf("%v", config.Datadog.GetInt("jmx_reconnection_timeout")), // Timeout for instance reconnection in seconds
		"--reconnection_thread_pool_size", fmt.Sprintf("%v", config.Datadog.GetInt("jmx_reconnection_thread_pool_size")), // Size for the JMXFetch reconnection thread pool
		"--log_level", jmxLogLevel,
		"--reporter", reporter, // Reporter to use
	)

	subprocessArgs = append(subprocessArgs, j.Command)

	if j.JmxExitFile != "" {
		exitFileDir := filepath.Join(here, "agent", "dist", "jmx", "run")
		os.MkdirAll(exitFileDir, os.ModeDir|0755)
		j.exitFilePath = filepath.Join(exitFileDir, j.JmxExitFile)
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

	err = j.cmd.Start()

	// start syncrhonization channels
	if err == nil && manage {
		j.managed = true
		j.shutdown = make(chan struct{})
		j.stopped = make(chan struct{})

		go j.Monitor()
	}

	return err
}

// Stop stops the JMXFetch process
func (j *JMXFetch) Stop() error {
	var stopChan chan struct{}

	if j.JmxExitFile == "" {
		// Unix
		err := j.cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			return err
		}

		if j.managed {
			stopChan = j.stopped
			close(j.shutdown)
		} else {
			stopChan = make(chan struct{})

			go func() {
				j.Wait()
				close(stopChan)
			}()
		}

		select {
		case <-time.After(time.Millisecond * 500):
			log.Warnf("Jmxfetch did not exit during it's grace period, killing it")
			err = j.cmd.Process.Signal(os.Kill)
			if err != nil {
				log.Warnf("Could not kill jmxfetch: %v", err)
			}
		case <-stopChan:
		}

	} else {
		// Windows
		if err := ioutil.WriteFile(j.exitFilePath, nil, 0644); err != nil {
			log.Warnf("Could not signal JMXFetch to exit, killing instead: %v", err)
			return j.cmd.Process.Kill()
		}
	}
	return nil
}

// Wait waits for the end of the JMXFetch process and returns the error code
func (j *JMXFetch) Wait() error {
	return j.cmd.Wait()
}

func (j *JMXFetch) heartbeat(beat *time.Ticker) {
	health := health.Register("jmxfetch")
	defer health.Deregister()

	for range beat.C {
		select {
		case <-health.C:
		case <-j.shutdown:
			return
		}
	}
}

// Up returns if JMXFetch is up - used by healthcheck
func (j *JMXFetch) Up() (bool, error) {
	// TODO: write windows implementation
	process, err := os.FindProcess(j.cmd.Process.Pid)
	if err != nil {
		return false, fmt.Errorf("Failed to find process: %s\n", err)
	}

	// from man kill(2):
	// if sig is 0, then no signal is sent, but error checking is still performed
	err = process.Signal(syscall.Signal(0))
	return err == nil, err
}
