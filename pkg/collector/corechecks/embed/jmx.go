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
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
	"gopkg.in/yaml.v2"
)

const (
	jmxJarName                        = "jmxfetch-0.14.0-jar-with-dependencies.jar"
	jmxMainClass                      = "org.datadog.jmxfetch.App"
	jmxCollectCommand                 = "collect"
	jvmDefaultMaxMemoryAllocation     = " -Xmx200m"
	jvmDefaultInitialMemoryAllocation = " -Xms50m"
	linkToDoc                         = "See http://docs.datadoghq.com/integrations/java/ for more information"
)

var jmxChecks = [...]string{
	"activemq",
	"activemq_58",
	"cassandra",
	"jmx",
	"solr",
	"tomcat",
	"kafka",
}

// Structures to parse the yaml containing a list of jmx checks config files
type instanceCfg struct {
	Files []string `yaml:"files"`
}

type jmxCfg struct {
	instance instanceCfg
}

// Structures to parse the config of an actual jmx check
type conf struct {
	Include map[string]interface{} `yaml:"include,omitempty"`
	Exclude map[string]interface{} `yaml:"exclude,omitempty"`
}

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
	Conf               []conf            `yaml:"conf,omitempty"`
}

type checkInitCfg struct {
	CustomJarPaths []string `yaml:"custom_jar_paths,omitempty"`
	ToolsJarPath   string   `yaml:"tools_jar_path,omitempty"`
	JavaBinPath    string   `yaml:"java_bin_path,omitempty"`
	JavaOptions    string   `yaml:"java_options,omitempty"`
	Conf           []conf   `yaml:"conf,omitempty"`
}

type checkCfg struct {
	InitConf  checkInitCfg       `yaml:"init_config,omitempty"`
	Instances []checkInstanceCfg `yaml:"instances,omitempty"`
}

func (cfg *jmxCfg) Parse(data []byte) error {

	if err := yaml.Unmarshal(data, &(cfg.instance)); err != nil {
		return err
	}

	if len(cfg.instance.Files) == 0 {
		return fmt.Errorf("Error parsing configuration: no config files")
	}

	return nil

}

func (cfg *checkCfg) Parse(data []byte) error {

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return err
	}

	return nil

}

func readJMXConf(checkConf *checkCfg, filename string) (
	javaBinPath string, javaOptions string, toolsJarPath string, customJarPaths []string, err error) {

	javaBinPath = checkConf.InitConf.JavaBinPath
	javaOptions = checkConf.InitConf.JavaOptions
	toolsJarPath = checkConf.InitConf.ToolsJarPath
	customJarPaths = checkConf.InitConf.CustomJarPaths
	isAttachAPI := false

	if len(checkConf.Instances) == 0 {
		return "", "", "", nil, fmt.Errorf("You need to have at least one instance " +
			"defined in the YAML file for this check")
	}

	for _, instance := range checkConf.Instances {
		// if these were not set in init config but in an instance, set to the first one encountered
		if toolsJarPath == "" && instance.ToolsJarPath != "" {
			toolsJarPath = instance.ToolsJarPath
		}
		if javaBinPath == "" && instance.JavaBinPath != "" {
			javaBinPath = instance.JavaBinPath
		}
		if javaOptions == "" && instance.JavaOptions != "" {
			javaOptions = instance.JavaOptions
		}

		if instance.ProcessNameRegex != "" {
			isAttachAPI = true
		} else if instance.JMXUrl != "" {
			if instance.Name == "" {
				return "", "", "", nil, fmt.Errorf("A name must be specified when using a jmx_url")
			}
		} else {
			if instance.Host == "" {
				return "", "", "", nil, fmt.Errorf("A host must be specified")
			}
			if instance.Port == 0 {
				return "", "", "", nil, fmt.Errorf("A numeric port must be specified")
			}
		}

		confs := instance.Conf
		if confs == nil {
			confs = checkConf.InitConf.Conf
		}

		if len(confs) == 0 {
			log.Warnf("%s doesn't have a 'conf' section. Only basic JVM metrics"+
				" will be collected. %s", filename, linkToDoc)
		} else {
			for _, conf := range confs {
				if len(conf.Include) == 0 {
					return "", "", "", nil, fmt.Errorf("Each configuration must have an"+
						" 'include' section. %s", linkToDoc)
				}
			}
		}
	}

	if isAttachAPI {
		if toolsJarPath == "" {
			return "", "", "", nil, fmt.Errorf("You must specify the path to tools.jar. %s", linkToDoc)
		} else if _, err := os.Open(toolsJarPath); err != nil {
			return "", "", "", nil, fmt.Errorf("Unable to find tools.jar at %s", toolsJarPath)
		}
	} else {
		toolsJarPath = ""
	}

	for _, path := range customJarPaths {
		if _, err := os.Open(path); err != nil {
			return "", "", "", nil, fmt.Errorf("Unable to find custom jar at %s", path)
		}
	}

	return javaBinPath, javaOptions, toolsJarPath, customJarPaths, nil
}

// JMXCheck keeps track of the running command
type JMXCheck struct {
	cmd          *exec.Cmd
	ExitFilePath string
}

func (c *JMXCheck) String() string {
	return "JMX Check"
}

// Run executes the check
func (c *JMXCheck) Run() error {

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

	if err = c.cmd.Start(); err != nil {
		return err
	}

	return c.cmd.Wait()
}

// Configure the JMXCheck
func (c *JMXCheck) Configure(data, initConfig check.ConfigData) error {
	jmxConfPath := path.Join(config.Datadog.GetString("confd_path"), "jmx")
	jmxConf := new(jmxCfg)

	jmxChecks := []string{}
	javaBinPath := ""
	javaOptions := ""
	toolsJarPath := ""
	customJarPaths := []string{}

	if err := jmxConf.Parse(data); err != nil {
		return err
	}

	for _, confFile := range jmxConf.instance.Files {
		checkConf := new(checkCfg)
		fpath := path.Join(jmxConfPath, confFile)

		// Read file contents
		// FIXME: ReadFile reads the entire file, possible security implications
		yamlFile, err := ioutil.ReadFile(fpath)
		if err != nil {
			return err
		}

		if err := checkConf.Parse(yamlFile); err != nil {
			log.Errorf("Unable to parse %s: %s", confFile, err)
			continue
		}

		if checkJavaBinPath, checkJavaOptions, checkToolsJarPath, checkCustomJarPaths, err := readJMXConf(checkConf, confFile); err == nil {
			jmxChecks = append(jmxChecks, confFile)
			if javaBinPath == "" && checkJavaBinPath != "" {
				javaBinPath = checkJavaBinPath
			}
			if javaOptions == "" && checkJavaOptions != "" {
				javaOptions = checkJavaOptions
			}
			if toolsJarPath == "" && checkToolsJarPath != "" {
				toolsJarPath = checkToolsJarPath
			}
			if checkCustomJarPaths != nil {
				customJarPaths = append(customJarPaths, checkCustomJarPaths...)
			}
		} else {
			log.Errorf("Invalid JMX configuration in file %s: %s", confFile, err)
			continue
		}
	}

	here, _ := osext.ExecutableFolder()
	jmxJarPath := path.Join(here, "dist", "jmx", jmxJarName)
	classpath := jmxJarPath
	if toolsJarPath != "" {
		classpath = fmt.Sprintf("%s:%s", toolsJarPath, classpath)
	}
	if len(customJarPaths) > 0 {
		classpath = fmt.Sprintf("%s:%s", strings.Join(customJarPaths, ":"), classpath)
	}
	bindHost := config.Datadog.GetString("bind_host")
	if bindHost == "" || bindHost == "0.0.0.0" {
		bindHost = "localhost"
	}
	reporter := fmt.Sprintf("statsd:%s:%s", bindHost, config.Datadog.GetString("dogstatsd_port"))

	//TODO : support auto discovery

	subprocessArgs := []string{}

	// Specify a maximum memory allocation pool for the JVM
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
		"--check_period", fmt.Sprintf("%v", int(check.DefaultCheckInterval/time.Millisecond)), // Period of the main loop of jmxfetch in ms
		"--conf_directory", jmxConfPath, // Path of the conf directory that will be read by jmxfetch,
		"--log_level", "INFO", //FIXME : Use agent log level when available
		"--log_location", path.Join(here, "dist", "jmx", "jmxfetch.log"), // FIXME : Path of the log file. At some point we should have a `run` folder
		"--reporter", reporter, // Reporter to use
		"--status_location", path.Join(here, "dist", "jmx", "jmx_status.yaml"), // FIXME : Path to the status file to write. At some point we should have a `run` folder
		jmxCollectCommand, // Name of the command
	)
	if len(jmxChecks) > 0 {
		subprocessArgs = append(subprocessArgs, "--check")
		subprocessArgs = append(subprocessArgs, jmxChecks...)
	} else {
		return fmt.Errorf("No valid JMX configuration found in %s", jmxConfPath)
	}

	if jmxExitFile != "" {
		c.ExitFilePath = path.Join(here, "dist", "jmx", jmxExitFile) // FIXME : At some point we should have a `run` folder
		// Signal handlers are not supported on Windows:
		// use a file to trigger JMXFetch exit instead
		subprocessArgs = append(subprocessArgs, "--exit_file_location", c.ExitFilePath)
	}

	if javaBinPath == "" {
		javaBinPath = "java"
	}
	c.cmd = exec.Command(javaBinPath, subprocessArgs...)

	return nil
}

// InitSender initializes a sender but we don't need any
func (c *JMXCheck) InitSender() {}

// Interval returns the scheduling time for the check, this will be scheduled only once
// since `Run` won't return, thus implementing a long running check.
func (c *JMXCheck) Interval() time.Duration {
	return 0
}

// ID returns the name of the check since there should be only one instance running
func (c *JMXCheck) ID() check.ID {
	return "JMX_Check"
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
}

func init() {
	factory := func() check.Check {
		return &JMXCheck{}
	}
	core.RegisterCheck("jmx", factory)
}
