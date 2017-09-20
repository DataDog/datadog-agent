// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

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

	api "github.com/DataDog/datadog-agent/cmd/agent/api/common"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
	"gopkg.in/yaml.v2"
)

const (
	jmxJarName                        = "jmxfetch-0.17.0-jar-with-dependencies.jar"
	jmxMainClass                      = "org.datadog.jmxfetch.App"
	jmxCollectCommand                 = "collect"
	jvmDefaultMaxMemoryAllocation     = " -Xmx200m"
	jvmDefaultInitialMemoryAllocation = " -Xms50m"
	linkToDoc                         = "See http://docs.datadoghq.com/integrations/java/ for more information"
)

type checkInstanceCfg struct {
	Host               string            `yaml:"host,omitempty"`
	Port               int               `yaml:"port,omitempty"`
	User               string            `yaml:"user,omitempty"`
	Password           string            `yaml:"password,omitempty"`
	JMXUrl             string            `yaml:"jmx_url,omitempty"`
	Name               string            `yaml:"name,omitempty"`
	JavaBinPath        string            `yaml:"java_bin_path,omitempty"`
	JavaOptions        string            `yaml:"java_options,omitempty"`
	ToolsJarPath       string            `yaml:"tools_jar_path,omitempty"`
	TrustStorePath     string            `yaml:"trust_store_path,omitempty"`
	TrustStorePassword string            `yaml:"trust_store_password,omitempty"`
	ProcessNameRegex   string            `yaml:"process_name_regex,omitempty"`
	RefreshBeans       int               `yaml:"refresh_beans,omitempty"`
	Tags               map[string]string `yaml:"tags,omitempty"`
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
}

// singleton for the JMXCheck
var jmxLauncher *JMXCheck

func (c *JMXCheck) String() string {
	return "JMX Check"
}

// Run executes the check
func (c *JMXCheck) Run() error {

	if len(c.checks) == 0 {
		return fmt.Errorf("No JMX checks configured - skipping.")
	}

	if atomic.LoadUint32(&c.running) == 1 {
		log.Info("JMX already running.")
		return nil
	}
	atomic.StoreUint32(&c.running, 1)

	here, _ := osext.ExecutableFolder()
	jmxConfPath := config.Datadog.GetString("confd_path")
	classpath := path.Join(here, "dist", "jmx", jmxJarName)
	if c.javaToolsJarPath != "" {
		classpath = fmt.Sprintf("%s:%s", c.javaToolsJarPath, classpath)
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

	subprocessArgs = append(subprocessArgs,
		"-classpath", classpath,
		jmxMainClass,
		"--ipc_port", fmt.Sprintf("%v", config.Datadog.GetInt("cmd_port")),
		"--check_period", fmt.Sprintf("%v", int(check.DefaultCheckInterval/time.Millisecond)), // Period of the main loop of jmxfetch in ms
		"--conf_directory", jmxConfPath, // Path of the conf directory that will be read by jmxfetch,
		"--log_level", "INFO", //FIXME : Use agent log level when available
		"--log_location", path.Join(here, "dist", "jmx", "jmxfetch.log"), // FIXME : Path of the log file. At some point we should have a `run` folder
		"--reporter", reporter, // Reporter to use
		jmxCollectCommand, // Name of the command
	)
	if len(c.checks) > 0 {
		subprocessArgs = append(subprocessArgs, "--check")
		for c, _ := range c.checks {
			subprocessArgs = append(subprocessArgs, c)
		}
	} else {
		log.Errorf("No valid JMX configuration found in %s", jmxConfPath)
	}

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
	if err = c.cmd.Start(); err != nil {
		return err
	}

	err = c.cmd.Wait()
	atomic.StoreUint32(&c.running, 0)

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
	if jmxExitFile == "" {
		err := c.cmd.Process.Signal(os.Kill)
		if err != nil {
			log.Errorf("unable to stop JMX check: %s", err)
		}
	} else {
		if err := ioutil.WriteFile(c.ExitFilePath, nil, 0644); err != nil {
			log.Errorf("unable to stop JMX check: %s", err)
		}
	}
	atomic.StoreUint32(&c.running, 0)
}

// GetWarnings does not return anything in JMX
func (c *JMXCheck) GetWarnings() []error {
	return []error{}
}

func init() {
	factory := func() check.Check {
		if jmxLauncher != nil {
			return jmxLauncher
		}
		jmxLauncher = &JMXCheck{checks: make(map[string]struct{})}
		return jmxLauncher
	}
	core.RegisterCheck("jmx", factory)
}
