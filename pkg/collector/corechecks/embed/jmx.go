package embed

import (
	"bufio"
	"errors"
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
	// "github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
	"gopkg.in/yaml.v2"
)

const jmxJarName = "jmxfetch-0.13.0-jar-with-dependencies.jar"
const jmxMainClass = "org.datadog.jmxfetch.App"
const jmxCollectCommand = "collect"
const jvmDefaultMaxMemoryAllocation = " -Xmx200m"
const jvmDefaultInitialMemoryAllocation = " -Xms50m"
const linkToDoc = "See http://docs.datadoghq.com/integrations/java/ for more information"

var jmxExitFilePath = ""

// const jvmDefaultSDMaxMemoryAllocation = " -Xmx512m"

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
		return errors.New("Error parsing configuration: no config files")
	}

	return nil

}

func (cfg *checkCfg) Parse(fpath string) error {

	// Read file contents
	// FIXME: ReadFile reads the entire file, possible security implications
	yamlFile, err := ioutil.ReadFile(fpath)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(yamlFile, cfg); err != nil {
		return err
	}

	return nil

}

func readJMXConf(checkConf *checkCfg, filename string) (string, string, string, []string, error) {
	javaBinPath := checkConf.InitConf.JavaBinPath
	javaOptions := checkConf.InitConf.JavaOptions
	toolsJarPath := checkConf.InitConf.ToolsJarPath
	customJarPaths := checkConf.InitConf.CustomJarPaths
	isAttachApi := false

	if len(checkConf.Instances) == 0 {
		return "", "", "", nil, errors.New("You need to have at least one instance " +
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
			isAttachApi = true
		} else if instance.JMXUrl != "" {
			if instance.Name == "" {
				return "", "", "", nil, errors.New("A name must be specified when using a jmx_url")
			}
		} else {
			if instance.Host == "" {
				return "", "", "", nil, errors.New("A host must be specified")
			}
			if instance.Port == 0 {
				return "", "", "", nil, errors.New("A numeric port must be specified")
			}
		}

		confs := instance.Conf
		if confs == nil {
			confs = checkConf.InitConf.Conf
		}

		if confs == nil {
			log.Warnf("%s doesn't have a 'conf' section. Only basic JVM metrics"+
				" will be collected. %s", filename)
		} else {
			if len(confs) == 0 {
				return "", "", "", nil, errors.New(fmt.Sprintf("'conf' section should be a list"+
					" of configurations %s", linkToDoc))
			}

			for _, conf := range confs {
				if len(conf.Include) == 0 {
					return "", "", "", nil, errors.New(fmt.Sprintf("Each configuration must have an"+
						" 'include' section. %s", linkToDoc))
				}
			}
		}
	}

	if isAttachApi {
		if toolsJarPath == "" {
			return "", "", "", nil, errors.New(fmt.Sprintf("You must specify the path to tools.jar. %s", linkToDoc))
		} else if _, err := os.Open(toolsJarPath); err != nil {
			return "", "", "", nil, errors.New(fmt.Sprintf("Unable to find tools.jar at %s", toolsJarPath))
		}
	} else {
		toolsJarPath = ""
	}

	for _, path := range customJarPaths {
		if _, err := os.Open(path); err != nil {
			return "", "", "", nil, errors.New(fmt.Sprintf("Unable to find custom jar at %s", path))
		}
	}

	fmt.Printf("%v\n", *checkConf)
	fmt.Printf("%s | %s | %s | %v\n", javaBinPath, javaOptions, toolsJarPath, customJarPaths)
	return javaBinPath, javaOptions, toolsJarPath, customJarPaths, nil
}

// JMXCheck keeps track of the running command
type JMXCheck struct {
	cmd *exec.Cmd
}

func (c *JMXCheck) String() string {
	return "JMX Check"
}

// Run executes the check
func (c *JMXCheck) Run() error {

	// remove the exit file trigger (windows)
	if jmxExitFile != "" {
		os.Remove(jmxExitFilePath)
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

		if err := checkConf.Parse(fpath); err != nil {
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
		classpath = fmt.Sprintf("%s:%s", classpath, jmxJarPath)
	}
	if len(customJarPaths) > 0 {
		classpath = fmt.Sprintf("%s:%s", classpath, strings.Join(customJarPaths, ":"))
	}
	bindHost := config.Datadog.GetString("bind_host")
	if bindHost == "" {
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
		"--check_period", fmt.Sprintf("%v", int(check.DefaultCheckInterval.Seconds()*1000)), // Period of the main loop of jmxfetch in ms
		"--conf_directory", jmxConfPath, // Path of the conf directory that will be read by jmxfetch,
		"--log_level", "INFO",
		"--log_location", path.Join(here, "dist", "jmx", "jmxfetch.log"), // Path of the log file
		"--reporter", reporter, // Reporter to use
		"--status_location", path.Join(here, "dist", "jmx", "jmx_status.yaml"), // Path to the status file to write
		jmxCollectCommand, // Name of the command
	)
	if len(jmxChecks) > 0 {
		subprocessArgs = append(subprocessArgs, "--check")
		subprocessArgs = append(subprocessArgs, jmxChecks...)
	} else {
		return errors.New(fmt.Sprintf("No valid JMX configuration found in %s", jmxConfPath))
	}

	if jmxExitFile != "" {
		jmxExitFilePath = path.Join(here, "dist", "jmx", jmxExitFile)
		// Signal handlers are not supported on Windows:
		// use a file to trigger JMXFetch exit instead
		subprocessArgs = append(subprocessArgs, "--exit_file_location", jmxExitFilePath)
	}

	if javaBinPath == "" {
		javaBinPath = "java"
	}
	fmt.Printf("%v", subprocessArgs)
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
		if err := ioutil.WriteFile(jmxExitFilePath, nil, 0644); err != nil {
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

// func getHostname() string {
// 	hname, found := util.Cache.Get(path.Join(util.AgentCachePrefix, "hostname"))
// 	if found {
// 		return hname.(string)
// 	}
// 	return ""
// }
