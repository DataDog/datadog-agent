// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package embed

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"
	yaml "gopkg.in/yaml.v2"

	api "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

const (
	jmxJarName                        = "jmxfetch-0.18.1-jar-with-dependencies.jar"
	jmxMainClass                      = "org.datadog.jmxfetch.App"
	jmxCollectCommand                 = "collect"
	jvmDefaultMaxMemoryAllocation     = " -Xmx200m"
	jvmDefaultInitialMemoryAllocation = " -Xms50m"
	linkToDoc                         = "See http://docs.datadoghq.com/integrations/java/ for more information"
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

// checkInstanceCfg lists the config options on the instance against which we make some sanity checks
// on how they're configured. All the other options should be checked on JMXFetch's side.
type checkInstanceCfg struct {
	JavaBinPath      string `yaml:"java_bin_path,omitempty"`
	JavaOptions      string `yaml:"java_options,omitempty"`
	ToolsJarPath     string `yaml:"tools_jar_path,omitempty"`
	ProcessNameRegex string `yaml:"process_name_regex,omitempty"`
}

type checkInitCfg struct {
	CustomJarPaths []string `yaml:"custom_jar_paths,omitempty"`
	ToolsJarPath   string   `yaml:"tools_jar_path,omitempty"`
	JavaBinPath    string   `yaml:"java_bin_path,omitempty"`
	JavaOptions    string   `yaml:"java_options,omitempty"`
}

// JMXCheck keeps track of the running command
type JMXCheck struct {
	cmd                *exec.Cmd
	checks             map[string]struct{}
	ExitFilePath       string
	javaBinPath        string
	javaOptions        string
	javaToolsJarPath   string
	javaCustomJarPaths []string
	isAttachAPI        bool
	running            uint32
	stop               chan struct{}
	stopDone           chan struct{}
}

var jmxLauncher = JMXCheck{
	checks:   make(map[string]struct{}),
	stop:     make(chan struct{}),
	stopDone: make(chan struct{}),
}

func (c *JMXCheck) String() string {
	return "JMX Check"
}

// Run executes the check with retries
func (c *JMXCheck) Run() error {
	if len(c.checks) == 0 {
		return fmt.Errorf("No JMX checks configured - skipping.")
	}

	atomic.StoreUint32(&c.running, 1)
	// TODO: retries should be configurable with meaningful default values
	err := check.Retry(defaultRetryDuration, defaultRetries, c.run, c.String())
	atomic.StoreUint32(&c.running, 0)

	return err
}

// run executes the check
func (c *JMXCheck) run() error {
	select {
	// poll the stop channel once to make sure no stop was requested since the last call to `run`
	case <-c.stop:
		log.Info("Not starting JMX: stop requested")
		c.stopDone <- struct{}{}
		return nil
	default:
	}

	err := c.start()
	if err != nil {
		return retryExitError(err)
	}

	processDone := make(chan error)
	go func() {
		processDone <- c.cmd.Wait()
	}()

	select {
	case err = <-processDone:
		return retryExitError(err)
	case <-c.stop:
		if jmxExitFile == "" {
			// Unix
			err = c.cmd.Process.Signal(os.Kill)
			if err != nil {
				log.Errorf("unable to stop JMX check: %s", err)
			}
		} else {
			// Windows
			if err = ioutil.WriteFile(c.ExitFilePath, nil, 0644); err != nil {
				log.Errorf("unable to stop JMX check: %s", err)
			}
		}
	}

	// wait for process to exit
	err = <-processDone
	c.stopDone <- struct{}{}
	return err
}

func (c *JMXCheck) Parse(data, initConfig check.ConfigData) error {

	var initConf checkInitCfg
	var instanceConf checkInstanceCfg

	// unmarshall instance info
	if err := yaml.Unmarshal(data, &instanceConf); err != nil {
		return err
	}

	// unmarshall init config
	if err := yaml.Unmarshal(initConfig, &initConf); err != nil {
		return err
	}

	if c.javaBinPath == "" {
		if instanceConf.JavaBinPath != "" {
			c.javaBinPath = instanceConf.JavaBinPath
		} else if initConf.JavaBinPath != "" {
			c.javaBinPath = initConf.JavaBinPath
		}
	}
	if c.javaOptions == "" {
		if instanceConf.JavaOptions != "" {
			c.javaOptions = instanceConf.JavaOptions
		} else if initConf.JavaOptions != "" {
			c.javaOptions = initConf.JavaOptions
		}
	}
	if c.javaToolsJarPath == "" {
		if instanceConf.ToolsJarPath != "" {
			c.javaToolsJarPath = instanceConf.ToolsJarPath
		} else if initConf.ToolsJarPath != "" {
			c.javaToolsJarPath = initConf.ToolsJarPath
		}
	}
	if c.javaCustomJarPaths == nil {
		if initConf.CustomJarPaths != nil {
			c.javaCustomJarPaths = initConf.CustomJarPaths
		}
	}

	if instanceConf.ProcessNameRegex != "" {
		if c.javaToolsJarPath == "" {
			return fmt.Errorf("You must specify the path to tools.jar. %s", linkToDoc)
		}
		c.isAttachAPI = true
	}

	return nil
}

// Configure the JMXCheck
func (c *JMXCheck) Configure(data, initConfig check.ConfigData) error {

	err := c.Parse(data, initConfig)
	if err != nil {
		return err
	}

	return nil
}

// Interval returns the scheduling time for the check, this will be scheduled only once
// since `Run` won't return, thus implementing a long running check.
func (c *JMXCheck) Interval() time.Duration {
	return 0
}

// ID returns the name of the check since there should be only one instance running
func (c *JMXCheck) ID() check.ID {
	return "JMX_Check"
}

// GetMetricStats returns the stats from the last run of the check, but there aren't any yet
func (c *JMXCheck) GetMetricStats() (map[string]int64, error) {
	return make(map[string]int64), nil
}

// Stop sends a termination signal to the JMXFetch process
func (c *JMXCheck) Stop() {
	if atomic.LoadUint32(&c.running) == 0 {
		log.Info("JMX not running.")
		return
	}

	c.stop <- struct{}{}
	<-c.stopDone
}

func (c *JMXCheck) start() error {
	here, _ := executable.Folder()
	classpath := path.Join(here, "dist", "jmx", jmxJarName)
	if c.javaToolsJarPath != "" {
		classpath = fmt.Sprintf("%s:%s", c.javaToolsJarPath, classpath)
	}

	globalCustomJars := config.Datadog.GetStringSlice("jmx_custom_jars")
	if len(globalCustomJars) > 0 {
		classpath = fmt.Sprintf("%s:%s", strings.Join(globalCustomJars, ":"), classpath)
	}

	if len(c.javaCustomJarPaths) > 0 {
		classpath = fmt.Sprintf("%s:%s", strings.Join(c.javaCustomJarPaths, ":"), classpath)
	}
	bindHost := config.Datadog.GetString("bind_host")
	if bindHost == "" || bindHost == "0.0.0.0" {
		bindHost = "localhost"
	}
	reporter := fmt.Sprintf("statsd:%s:%s", bindHost, config.Datadog.GetString("dogstatsd_port"))

	//TODO : support auto discovery

	subprocessArgs := []string{}

	// Specify a maximum memory allocation pool for the JVM
	javaOptions := c.javaOptions
	if !strings.Contains(javaOptions, "Xmx") && !strings.Contains(javaOptions, "XX:MaxHeapSize") {
		javaOptions += jvmDefaultMaxMemoryAllocation
	}
	// Specify the initial memory allocation pool for the JVM
	if !strings.Contains(javaOptions, "Xms") && !strings.Contains(javaOptions, "XX:InitialHeapSize") {
		javaOptions += jvmDefaultInitialMemoryAllocation
	}

	subprocessArgs = append(subprocessArgs, strings.Fields(javaOptions)...)

	jmxLogLevel, ok := jmxLogLevelMap[strings.ToLower(config.Datadog.GetString("log_level"))]
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
		"--log_location", path.Join(here, "dist", "jmx", "jmxfetch.log"), // FIXME : Path of the log file. At some point we should have a `run` folder
		"--reporter", reporter, // Reporter to use
		jmxCollectCommand, // Name of the command
	)

	if jmxExitFile != "" {
		c.ExitFilePath = path.Join(here, "dist", "jmx", jmxExitFile) // FIXME : At some point we should have a `run` folder
		// Signal handlers are not supported on Windows:
		// use a file to trigger JMXFetch exit instead
		subprocessArgs = append(subprocessArgs, "--exit_file_location", c.ExitFilePath)
	}

	javaBinPath := c.javaBinPath
	if javaBinPath == "" {
		javaBinPath = "java"
	}
	c.cmd = exec.Command(javaBinPath, subprocessArgs...)

	// set environment + token
	c.cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("SESSION_TOKEN=%s", api.GetAuthToken()),
	)

	// remove the exit file trigger (windows)
	if jmxExitFile != "" {
		os.Remove(c.ExitFilePath)
	}

	// forward the standard output to the Agent logger
	stdout, err := c.cmd.StdoutPipe()
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
	stderr, err := c.cmd.StderrPipe()
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

	return c.cmd.Start()
}

// GetWarnings does not return anything in JMX
func (c *JMXCheck) GetWarnings() []error {
	return []error{}
}
